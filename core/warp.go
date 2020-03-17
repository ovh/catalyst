package core

import (
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var mads *prometheus.CounterVec
var ddp *prometheus.CounterVec
var brokenPipe prometheus.Counter
var httpClient *http.Client
var httpClientSingleton = sync.Once{}
var warpEndpoint string

// Warp connection
type Warp struct {
	token string
	pr    *io.PipeReader
	pw    *io.PipeWriter
	err   error
	mutex sync.Mutex
	close sync.Mutex
}

// GTS struct
type GTS struct {
	Ts     float64
	Name   string
	Labels map[string]string
	Value  interface{}
}

// WarpInvalidToken invalid warp token error
type WarpInvalidToken struct {
	Token string
}

func (e WarpInvalidToken) Error() string {
	return fmt.Sprintf("Invalid token: %v", e.Token)
}

// WarpExpiredToken expired warp token error
type WarpExpiredToken struct {
	Token string
}

func (e WarpExpiredToken) Error() string {
	return fmt.Sprintf("Token expired: %s", e.Token)
}

// WarpRevokedToken represent a revoked warp token error
type WarpRevokedToken struct {
	Token string
}

func (e WarpRevokedToken) Error() string {
	return fmt.Sprintf("Revoked token: %v", e.Token)
}

// WarpMadsExceeded mads exceeded error
type WarpMadsExceeded struct {
	App   string
	Limit string
	Body  string
}

func (e WarpMadsExceeded) Error() string {
	return fmt.Sprintf("MADS exceeded: %v", e.Limit)
}

// WarpDDPExceeded mads exceeded error
type WarpDDPExceeded struct {
	App   string
	Limit string
	Body  string
}

func (e WarpDDPExceeded) Error() string {
	return fmt.Sprintf("DDP exceeded: %v", e.Limit)
}

// WarpInputError invalid input error
type WarpInputError struct {
	Str  string
	Body string
}

// WarpGoneError invalid application error
type WarpGoneError struct {
	Str  string
	Body string
}

func (e WarpInputError) Error() string {
	return fmt.Sprintf("Invalid input: %v", e.Str)
}

func (e WarpGoneError) Error() string {
	return fmt.Sprintf("Invalid application: %v", e.Str)
}

// NewWarp returns a Warp connection.
func NewWarp(token, txn, now string) (*Warp, error) {
	httpClientSingleton.Do(func() {
		httpClient = &http.Client{
			Timeout: viper.GetDuration("warp.connection.timeout"),
			Transport: &http.Transport{
				DisableKeepAlives: false,
				Dial: (&net.Dialer{
					Timeout: viper.GetDuration("warp.connection.dial.timeout"),
				}).Dial,
				TLSHandshakeTimeout: viper.GetDuration("warp.connection.tls.timeout"),
				MaxIdleConnsPerHost: viper.GetInt("warp.connection.idle.max"),
				IdleConnTimeout:     viper.GetDuration("warp.connection.keep-alive.timeout"),
			},
		}

		warpEndpoint = viper.GetString("warp_endpoint")

		// Declare Prom metrics
		mads = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "catalyst",
			Subsystem: "error",
			Name:      "mads",
			Help:      "Mads errors.",
		}, []string{"app"})
		prometheus.MustRegister(mads)

		ddp = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "catalyst",
			Subsystem: "error",
			Name:      "ddp",
			Help:      "DDP errors.",
		}, []string{"app"})
		prometheus.MustRegister(ddp)

		brokenPipe = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "catalyst",
			Subsystem: "error",
			Name:      "broken_pipe",
			Help:      "Warp broken pipes errors",
		})
		prometheus.MustRegister(brokenPipe)
	})
	pr, pw := io.Pipe()

	w := &Warp{
		token: token,
		pr:    pr,
		pw:    pw,
	}

	req, err := http.NewRequest("POST", warpEndpoint+"/api/v0/update", pr)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("X-Warp10-Token", token)
	req.Header.Set("X-CityzenData-Token", token)
	if now != "" {
		req.Header.Set("X-Warp10-Now", now)
	}
	req.Header.Set("Txn", txn)

	go func() {
		w.close.Lock()
		defer w.close.Unlock()

		res, err := httpClient.Do(req)
		if err != nil {
			w.mutex.Lock()
			defer w.mutex.Unlock()
			w.err = err
			return
		}
		defer func() {
			if err := res.Body.Close(); err != nil {
				log.WithError(err).Error("Cannot close response body")
			}
		}()

		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			w.mutex.Lock()
			defer w.mutex.Unlock()
			w.err = err
			return
		}

		if res.StatusCode != 200 {
			w.mutex.Lock()
			defer w.mutex.Unlock()
			w.err = fmt.Errorf("status %v - %v", res.StatusCode, string(body))
			return
		}
	}()

	// hacky to ensure go send http header on the connection
	// else it only established tcp session without sending any data
	// causing 408 errors on IPLB side
	_, _ = w.pw.Write([]byte("#\r\n"))

	return w, nil
}

// Send a GTS.
func (w *Warp) Send(gts []byte) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.err != nil {
		return w.HandleError(w.err)
	}

	w.mutex.Unlock()
	log.Debugf("Series to send: %s", string(gts))
	_, _ = w.pw.Write(gts)
	w.mutex.Lock()

	return w.HandleError(w.err)
}

// Close the connection.
func (w *Warp) Close() error {
	err := w.pw.Close()

	w.close.Lock()
	defer w.close.Unlock()

	if err != nil {
		return w.HandleError(err)
	}

	return w.HandleError(w.err)
}

// HandleError read http error body and return a corresponding error
func (w *Warp) HandleError(e error) error {
	if e == nil {
		return nil
	}

	body := strings.ReplaceAll(e.Error(), "\n", " ")

	if strings.Contains(e.Error(), "io.warp10.script.WarpScriptException: Invalid token") {
		return WarpInvalidToken{
			Token: w.token,
		}
	}

	if strings.Contains(e.Error(), "io.warp10.script.WarpScriptException: Write token missing") {
		return WarpInvalidToken{
			Token: "Write token missing",
		}
	}

	if strings.Contains(e.Error(), "io.warp10.script.WarpScriptException: Token Expired") {
		return WarpExpiredToken{
			Token: w.token,
		}
	}

	if strings.Contains(e.Error(), "io.warp10.script.WarpScriptException: Token revoked") {
		return WarpRevokedToken{
			Token: w.token,
		}
	}

	if strings.Contains(e.Error(), "exceed your Monthly Active Data Streams limit") || strings.Contains(e.Error(), "exceed the Monthly Active Data Streams limit") {
		// register mads metric

		reg := regexp.MustCompile(`Monthly Active Data Streams limit(?: for application (?:&apos;|.)([^\(]*?)(?:&apos;|.)) \((\d+)(.\d+)?(E-\d)?\). \(Geo Time Series`)
		parts := reg.FindStringSubmatch(body)
		limit := "-1"
		app := ""
		if len(parts) > 1 {
			app = parts[1]
		}
		if len(parts) > 2 {
			limit = parts[2]
		}

		mads.With(prometheus.Labels{"app": app}).Inc()

		return WarpMadsExceeded{
			Limit: limit,
			Body:  body,
			App:   app,
		}
	}

	if strings.Contains(e.Error(), "Daily Data Points limit being already exceeded") {

		reg := regexp.MustCompile(`(?U)(,|{)\.app=(.*)(,|})`)
		parts := reg.FindStringSubmatch(e.Error())
		app := ""
		if len(parts) > 2 {
			app = parts[2]
		}

		reg = regexp.MustCompile(`Current maximum rate is \((\d+)(.\d+)?(E-\d)?\) datapoints/s`)
		parts = reg.FindStringSubmatch(e.Error())
		limit := "-1"
		if len(parts) > 1 {
			body = parts[0]
			limit = parts[1]
		}

		ddp.With(prometheus.Labels{"app": app}).Inc()

		return WarpDDPExceeded{
			Limit: limit,
			Body:  body,
			App:   app,
		}
	}

	if strings.Contains(e.Error(), "Parse error at") {
		reg := regexp.MustCompile(`<pre>\s*Parse error at &apos;(.*)&apos;</pre>`)
		parts := reg.FindStringSubmatch(e.Error())
		str := ""
		if len(parts) > 1 {
			str = parts[1]
		}

		return WarpInputError{
			Str:  str,
			Body: body,
		}
	}

	if strings.Contains(e.Error(), "Application suspended or closed") {
		return WarpGoneError{
			Str:  "Application suspended or closed",
			Body: body,
		}
	}

	if strings.Contains(e.Error(), "Parse error at") {
		reg := regexp.MustCompile(`<pre>\s*Parse error at &apos;(.*)&apos;</pre>`)
		parts := reg.FindStringSubmatch(e.Error())
		str := ""
		if len(parts) > 1 {
			str = parts[1]
		}

		return WarpInputError{
			Str:  str,
			Body: body,
		}
	}

	if strings.Contains(e.Error(), "For input string") {
		reg := regexp.MustCompile(`<pre>\s*For input string: &quot;(.*)&quot;</pre>`)
		parts := reg.FindStringSubmatch(e.Error())
		str := ""
		if len(parts) > 1 {
			str = parts[1]
		}

		return WarpInputError{
			Str:  str,
			Body: body,
		}
	}

	if strings.Contains(e.Error(), "broken pipe") {
		brokenPipe.Inc()
	}

	return e
}

// Encode a GTS to the Sensision format
// TS/LAT:LON/ELEV NAME{LABELS} VALUE
func (gts *GTS) Encode() []byte {
	sensision := ""

	// Timestamp
	if !math.IsNaN(gts.Ts) {
		sensision += fmt.Sprintf("%d", int(gts.Ts))
	}

	// Class
	// In case of an URLENCODED string, "+" are not converted anymore to spaces since Warp10 2.3.0
	sensision += fmt.Sprintf("// %s{", strings.ReplaceAll(url.QueryEscape(gts.Name), "+", "%20"))

	sep := ""
	for k, v := range gts.Labels {

		// In case of an URLENCODED labels, "+" are not converted anymore to spaces since Warp10 2.3.0
		sensision += sep + strings.ReplaceAll(url.QueryEscape(k)+"="+url.QueryEscape(v), "+", "%20")
		sep = ","
	}
	sensision += "} "

	// value
	switch gts.Value.(type) {
	case bool:
		if gts.Value.(bool) {
			sensision += "T"
		} else {
			sensision += "F"
		}

	case float64:
		sensision += fmt.Sprintf("%f", gts.Value.(float64))

	case int64:
		sensision += fmt.Sprintf("%d", gts.Value.(int64))

	case float32:
		sensision += fmt.Sprintf("%f", gts.Value.(float32))

	case int:
		sensision += fmt.Sprintf("%d", gts.Value.(int))

	case string:
		sensision += fmt.Sprintf("'%s'", url.QueryEscape(gts.Value.(string)))

	default:
		// Other types: just output their default format
		strVal := fmt.Sprintf("%v", gts.Value)
		sensision += url.QueryEscape(strVal)
	}
	sensision += "\r\n"

	return []byte(sensision)
}
