package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/golang/snappy"
	"github.com/labstack/echo"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	tokenSrv "github.com/ovh/catalyst/services/token"
)

var conResetCounter prometheus.Counter

func init() {
	conResetCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "catalyst",
		Subsystem: "error",
		Name:      "connreset",
		Help:      "Number of connections reset",
	})
	prometheus.MustRegister(conResetCounter)
}

// ParsingError parsing error
type ParsingError struct {
	Msg string
	Row string
}

func (h ParsingError) Error() string {
	return fmt.Sprintf("%v\n%v", h.Msg, h.Row)
}

// NewParsingError parsing error
func NewParsingError(msg, row string) ParsingError {
	return ParsingError{
		Msg: msg,
		Row: row,
	}
}

// Handler struct
type Handler struct {
	protocol     string
	methods      string
	handler      func(*url.URL, *http.Header, io.Reader, func([]byte) error, prometheus.Counter) (int, int, error)
	errorHandler func(error) error

	reqCounter prometheus.Counter
	errCounter prometheus.CounterVec
	dpCounter  prometheus.Counter
}

// NewHandler initialise a new api endpoint handler
func NewHandler(protocol string, methods []string, handler func(*url.URL, *http.Header, io.Reader, func([]byte) error, prometheus.Counter) (int, int, error), errorHandler func(error) error) *Handler {
	// metrics
	reqCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   "catalyst",
		Subsystem:   "protocol",
		Name:        "request",
		Help:        "Number of request handled.",
		ConstLabels: prometheus.Labels{"protocol": protocol},
	})
	prometheus.MustRegister(reqCounter)

	errCounter := *prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   "catalyst",
		Subsystem:   "protocol",
		Name:        "status_code",
		Help:        "Number of request in warning.",
		ConstLabels: prometheus.Labels{"protocol": protocol},
	}, []string{"status"})
	prometheus.MustRegister(errCounter)

	dpCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   "catalyst",
		Subsystem:   "protocol",
		Name:        "datapoints",
		Help:        "Number of processed datapoints.",
		ConstLabels: prometheus.Labels{"protocol": protocol},
	})
	prometheus.MustRegister(dpCounter)

	return &Handler{
		protocol:     protocol,
		methods:      strings.Join(methods, ":"),
		handler:      handler,
		errorHandler: errorHandler,

		reqCounter: reqCounter,
		errCounter: errCounter,
		dpCounter:  dpCounter,
	}
}

// Handle returns a handler.
func (h *Handler) Handle(c echo.Context) error {
	var err error
	datapoints := 0
	code := 0
	msg := ""
	req := c.Request()

	defer func() {
		if code == 0 {
			code = http.StatusOK
		}
		if err := c.String(code, msg); err != nil {
			log.WithError(err).Warn("Failed to answer client request")
		}

		c.Set("datapoints", datapoints)
	}()
	h.reqCounter.Inc()

	if !strings.Contains(h.methods, req.Method) {
		code = http.StatusMethodNotAllowed
		return nil
	}

	token, err := GetToken(req)
	if err != nil {
		code = http.StatusUnauthorized
		log.WithFields(log.Fields{
			"txn": c.Get("txn"),
		}).Warn("Bad token")
		h.errCounter.With(prometheus.Labels{
			"status": strconv.Itoa(code),
		}).Inc()
		return nil
	}

	r, err := handleGzip(req)
	if err != nil {
		code = http.StatusUnprocessableEntity
		log.WithFields(log.Fields{
			"txn":  c.Get("txn"),
			"code": code,
		}).Warn("Fail to decode gzip")
		msg = "Fail to decode gzip"
		h.errCounter.With(prometheus.Labels{
			"status": strconv.Itoa(code),
		}).Inc()
		return nil
	}

	if viper.GetBool("dryrun") {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(r)
		b := buf.Bytes()
		s := *(*string)(unsafe.Pointer(&b))
		fmt.Println(s)
	} else {

		// Get warp connection
		warp, err := NewWarp(token, c.Get("txn").(string), c.Request().Header.Get("X-Warp10-Now"))
		if err != nil {
			code = http.StatusBadGateway
			log.WithFields(log.Fields{
				"txn":  c.Get("txn"),
				"code": code,
			}).Error(err)
			h.errCounter.With(prometheus.Labels{
				"status": strconv.Itoa(code),
			}).Inc()
			return nil
		}

		// handle request
		datapoints, code, err = h.handler(req.URL, &req.Header, r, warp.Send, h.dpCounter)

		if err != nil {
			if h.errorHandler != nil {
				err = h.errorHandler(err)
			}
			code, msg = h.handleErr(req, err, c.Get("txn").(string))
		}

		if err = warp.Close(); err != nil {
			if h.errorHandler != nil {
				err = h.errorHandler(err)
			}
			code, msg = h.handleErr(req, err, c.Get("txn").(string))
			log.WithError(err).WithFields(log.Fields{
				"txn":  c.Get("txn").(string),
				"code": code,
			}).Warn("Fail to close connection")
		}

	}

	return nil
}

// handleErr try to map an error to right error code
func (h *Handler) handleErr(req *http.Request, err error, txn string) (int, string) {
	var code int

	if terr, ok := err.(WarpInvalidToken); ok {
		code = http.StatusUnauthorized
		log.WithError(terr).WithFields(log.Fields{
			"txn":  txn,
			"code": code,
		}).Warn("Bannish token: invalid token")
		h.errCounter.With(prometheus.Labels{
			"status": strconv.Itoa(code),
		}).Inc()
		tokenSrv.Bannish(terr.Token)
		return code, ""
	}

	if terr, ok := err.(WarpExpiredToken); ok {
		code = http.StatusUnauthorized
		log.WithError(terr).WithFields(log.Fields{
			"txn":  txn,
			"code": code,
		}).Warn("Bannish token: token expired")
		h.errCounter.With(prometheus.Labels{
			"status": strconv.Itoa(code),
		}).Inc()
		tokenSrv.Bannish(terr.Token)
		return code, ""
	}

	if terr, ok := err.(WarpRevokedToken); ok {
		code = http.StatusUnauthorized
		log.WithError(terr).WithFields(log.Fields{
			"txn":  txn,
			"code": code,
		}).Warn("Bannish token: token revoked")
		h.errCounter.With(prometheus.Labels{
			"status": strconv.Itoa(code),
		}).Inc()
		tokenSrv.Bannish(terr.Token)
		return code, ""
	}

	if merr, ok := err.(WarpMadsExceeded); ok {
		code = http.StatusTooManyRequests
		log.WithFields(log.Fields{
			"txn":   txn,
			"body":  merr.Body,
			"app":   merr.App,
			"limit": merr.Limit,
			"code":  code,
		}).Warn(merr)
		h.errCounter.With(prometheus.Labels{
			"status": strconv.Itoa(code),
		}).Inc()

		return code, merr.Error()
	}

	if derr, ok := err.(WarpDDPExceeded); ok {
		code = http.StatusTooManyRequests
		log.WithFields(log.Fields{
			"txn":   txn,
			"body":  derr.Body,
			"app":   derr.App,
			"limit": derr.Limit,
			"code":  code,
		}).Warn(derr)
		h.errCounter.With(prometheus.Labels{
			"status": strconv.Itoa(code),
		}).Inc()

		return code, derr.Error()
	}

	if ierr, ok := err.(WarpInputError); ok {
		code = http.StatusUnprocessableEntity
		log.WithFields(log.Fields{
			"txn":    txn,
			"body":   ierr.Body,
			"metric": ierr.Str,
			"code":   code,
		}).Error(ierr)
		h.errCounter.With(prometheus.Labels{
			"status": strconv.Itoa(code),
		}).Inc()

		return code, ierr.Error()
	}

	if ierr, ok := err.(WarpGoneError); ok {
		code = http.StatusGone
		log.WithFields(log.Fields{
			"txn":    txn,
			"body":   ierr.Body,
			"metric": ierr.Str,
			"code":   code,
		}).Error(ierr)
		h.errCounter.With(prometheus.Labels{
			"status": strconv.Itoa(code),
		}).Inc()

		return code, ierr.Error()
	}

	if ierr, ok := err.(InfluxDBParseError); ok {
		code = http.StatusBadRequest
		log.WithFields(log.Fields{
			"ip":   req.Header.Get("X-Forwarded-For"),
			"from": viper.GetString("hostname"),
			"txn":  txn,
			"code": code,
		}).Warn(ierr)
		h.errCounter.With(prometheus.Labels{
			"status": strconv.Itoa(code),
		}).Inc()

		err, _ := json.Marshal(ierr)
		return code, fmt.Sprintf("{\"error\":\"%s\"", err)
	}

	if perr, ok := err.(ParsingError); ok {
		code = http.StatusUnprocessableEntity
		log.WithFields(log.Fields{
			"txn":    txn,
			"metric": perr.Row,
			"code":   code,
		}).Warn(perr)
		h.errCounter.With(prometheus.Labels{
			"status": strconv.Itoa(code),
		}).Inc()

		return code, perr.Error()
	}

	if strings.Contains(err.Error(), "request canceled (Client.Timeout exceeded while awaiting headers)") {
		code = http.StatusRequestTimeout
		log.WithFields(log.Fields{
			"txn":    txn,
			"metric": "client.timeout",
			"code":   code,
		}).Warn(err)
		h.errCounter.With(prometheus.Labels{
			"status": strconv.Itoa(code),
		}).Inc()
		return code, err.Error()
	}

	if strings.Contains(err.Error(), "<title>Error 503: server unavailable</title>") {
		code = http.StatusServiceUnavailable
		log.WithFields(log.Fields{
			"txn":    txn,
			"metric": "service.unavailable",
			"code":   code,
		}).Warn(err)
		h.errCounter.With(prometheus.Labels{
			"status": strconv.Itoa(code),
		}).Inc()
		return code, err.Error()
	}

	if strings.Contains(err.Error(), "408 Request Time-out") {
		code = http.StatusRequestTimeout
		log.WithFields(log.Fields{
			"txn":    txn,
			"metric": "client.timeout",
			"code":   code,
		}).Warn(err)
		h.errCounter.With(prometheus.Labels{
			"status": strconv.Itoa(code),
		}).Inc()
		return code, err.Error()
	}

	if strings.Contains(err.Error(), "transport connection broken") {
		code = http.StatusRequestTimeout
		log.WithFields(log.Fields{
			"txn":    txn,
			"metric": "client.timeout",
			"code":   code,
		}).Warn(err)
		h.errCounter.With(prometheus.Labels{
			"status": strconv.Itoa(code),
		}).Inc()
		return code, err.Error()
	}

	if strings.Contains(err.Error(), "EOF") {
		code = http.StatusUnprocessableEntity
		log.WithFields(log.Fields{
			"txn":    txn,
			"metric": "eof",
			"code":   code,
		}).Warn(err)
		h.errCounter.With(prometheus.Labels{
			"status": strconv.Itoa(code),
		}).Inc()

		return code, err.Error()
	}

	if err == io.EOF {
		code = http.StatusUnprocessableEntity
		log.WithFields(log.Fields{
			"txn":    txn,
			"metric": "eof",
			"code":   code,
		}).Warn(err)
		h.errCounter.With(prometheus.Labels{
			"status": strconv.Itoa(code),
		}).Inc()

		return code, err.Error()
	}

	// https://github.com/golang/snappy/blob/master/decode.go#L15
	if err == snappy.ErrCorrupt || err == snappy.ErrTooLarge || err == snappy.ErrUnsupported {
		code = http.StatusUnprocessableEntity
		log.WithError(err).WithFields(log.Fields{
			"txn":  txn,
			"code": code,
		}).Error("could not parse prometheus remote write data")
		h.errCounter.With(prometheus.Labels{
			"status": strconv.Itoa(code),
		}).Inc()

		return code, err.Error()
	}

	// if con reset
	if ue, ok := err.(*url.Error); ok {
		if oe, ok := ue.Err.(*net.OpError); ok {
			if se, ok := oe.Err.(*os.SyscallError); ok {
				if errno, ok := se.Err.(syscall.Errno); ok {
					if errno == syscall.ECONNRESET {
						conResetCounter.Inc()
					}
				}
			}
		}
	}

	log.WithFields(log.Fields{
		"txn": txn,
	}).Error(err)
	h.errCounter.With(prometheus.Labels{
		"status": strconv.Itoa(http.StatusBadGateway),
	}).Inc()

	return http.StatusBadGateway, err.Error()
}
