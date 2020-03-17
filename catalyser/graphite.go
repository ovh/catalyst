package catalyser

import (
	"bufio"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ovh/catalyst/core"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

// GraphiteHTTP returns a Graphite catalyser.
func GraphiteHTTP(url *url.URL, header *http.Header, r io.Reader, send func([]byte) error, dpCounter prometheus.Counter) (int, int, error) {
	dps := 0
	txn := header.Get("X-App-Txn")

	// Get the stream in
	scan := bufio.NewReader(r)
	for {
		buf, _, err := scan.ReadLine()

		// End case
		if err == io.EOF {
			return dps, http.StatusAccepted, nil
		}

		if err != nil {
			log.WithFields(log.Fields{
				"txn":   txn,
				"error": err,
				"buf":   buf,
			}).Info("unable to read HTTP payload")
			return dps, http.StatusUnprocessableEntity, core.NewParsingError("unable to read HTTP payload", string(buf))
		}

		linePayload := strings.TrimSpace(string(buf))

		datapoint, err := parseLine(linePayload, true)

		if err != nil {
			log.WithFields(log.Fields{
				"txn":   txn,
				"error": err,
				"buf":   buf,
			}).Info("Failed to parse datapoint")
			return dps, http.StatusUnprocessableEntity, core.NewParsingError("Failed to parse datapoint", linePayload)
		}

		// Send to Warp
		err = send(datapoint.Encode())
		if err != nil {
			return dps, http.StatusInternalServerError, err
		}

		log.Debug(datapoint)

		dpCounter.Inc()
		dps++
	}
}

// Graphite is a Graphite socket who parse to sensision format
type Graphite struct {
	Listen string
	Parse  bool

	ReqTCPCounter       prometheus.Counter
	ReqTCPOKCounter     prometheus.Counter
	ReqTCPErrorCounter  prometheus.Counter
	ReqTCPNoAuthCounter prometheus.Counter
	ReqTCPdp            prometheus.Counter
	ReqTimes            prometheus.Counter
}

// NewGraphite return a new Graphite initialized with his output chan
func NewGraphite(listen string, p bool) *Graphite {
	graphite := &Graphite{
		Listen: listen,
		Parse:  p,
	}

	graphite.ReqTCPCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "catalyst",
		Subsystem: "graphite_tcp",
		Name:      "requests_total",
		Help:      "Number of request handled.",
	})

	prometheus.MustRegister(graphite.ReqTCPCounter)

	graphite.ReqTCPOKCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "catalyst",
		Subsystem: "graphite_tcp",
		Name:      "requests_success",
		Help:      "Number of request in success.",
	})

	prometheus.MustRegister(graphite.ReqTCPOKCounter)

	graphite.ReqTCPErrorCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "catalyst",
		Subsystem: "graphite_tcp",
		Name:      "requests_errors",
		Help:      "Number of request in errors.",
	})

	prometheus.MustRegister(graphite.ReqTCPErrorCounter)

	graphite.ReqTCPNoAuthCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "catalyst",
		Subsystem: "graphite_tcp",
		Name:      "requests_noauth",
		Help:      "Number of request where authentication is missing.",
	})

	prometheus.MustRegister(graphite.ReqTCPNoAuthCounter)

	graphite.ReqTCPdp = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "catalyst",
		Subsystem: "graphite_tcp",
		Name:      "requests_datapoints",
		Help:      "Number of datapoints handled.",
	})

	prometheus.MustRegister(graphite.ReqTCPdp)

	graphite.ReqTimes = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "catalyst",
		Subsystem: "graphite_tcp",
		Name:      "requests_elapsed_time",
		Help:      "Time took by each requests",
	})

	prometheus.MustRegister(graphite.ReqTimes)

	return graphite
}

// OpenTCPServer opens the Graphite TCP input format and starts processing data.
func (g *Graphite) OpenTCPServer() {
	ln, err := net.Listen("tcp", g.Listen)
	if err != nil {
		log.WithError(err).Fatalf("cannot open graphite TCP listener (%s)", g.Listen)
		return
	}

	log.Infof("TCP Listen on %s", g.Listen)

	for {
		conn, err := ln.Accept()

		if opErr, ok := err.(*net.OpError); ok && !opErr.Temporary() {
			log.WithFields(log.Fields{
				"error": opErr,
			}).Debug("Graphite TCP listener closed")
			continue
		}

		if err != nil {
			log.WithFields(log.Fields{
				"error": err.Error(),
			}).Warn("Error has occurred while accepting the TCP connection")
			continue
		}

		go g.handleTCPConnection(conn)
	}
}

// handleTCPConnection services an individual TCP connection for the Graphite input
func (g *Graphite) handleTCPConnection(conn net.Conn) {

	g.ReqTCPCounter.Inc()
	txn := fmt.Sprintf("%x", sha256.New().Sum(nil))
	warp := &core.Warp{}
	now := time.Now()

	defer func(txn string) {
		if err := conn.Close(); err != nil {
			log.WithFields(log.Fields{
				"txn": txn,
			}).WithError(err).Error("Cannot close the TCP request")
		}
	}(txn)

	reqDp := 0.0

	var metric string
	reader := bufio.NewReader(conn)
	hasToken := false
	tokenLength := 0
	for {
		buf, _, err := reader.ReadLine()

		// End case
		if err == io.EOF {

			if hasToken {
				if err := warp.Close(); err != nil {
					g.ReqTCPErrorCounter.Inc()
					log.WithFields(log.Fields{
						"txn":   txn,
						"error": err,
					}).Info("Failed to close warp client")

					elapsed := float64(time.Since(now))
					g.ReqTimes.Add(elapsed)
					return
				}
			}
			g.ReqTCPdp.Add(reqDp)
			g.ReqTCPOKCounter.Inc()

			elapsed := float64(time.Since(now))
			g.ReqTimes.Add(elapsed)
			return
		}

		if err != nil {
			g.ReqTCPErrorCounter.Inc()
			log.WithFields(log.Fields{
				"txn":   txn,
				"error": err,
				"buf":   buf,
			}).Warn("unable to read TCP payload")
			return
		}

		linePayload := strings.TrimSpace(string(buf))

		if !hasToken {

			if !strings.Contains(linePayload, "@.") {
				g.ReqTCPNoAuthCounter.Inc()
				return
			}

			splits := strings.Split(linePayload, "@.")

			if splits[0] == "" {
				g.ReqTCPNoAuthCounter.Inc()
				return
			}

			tokenLength = len(splits[0]) + 2
			hasToken = true

			warp, err = g.OpenWarp(splits[0], txn)
			if err != nil {
				g.ReqTCPErrorCounter.Inc()
				log.WithFields(log.Fields{
					"error":  err,
					"txn":    txn,
					"metric": metric,
				}).Info("unable to open warp 10 connection")
				return
			}
		}

		if len(linePayload) <= tokenLength {
			continue
		}

		metric = linePayload[tokenLength:]
		datapoint, err := parseLine(metric, g.Parse)

		if err != nil {
			log.WithFields(log.Fields{
				"error":  err,
				"txn":    txn,
				"metric": metric,
			}).Info("unable to parse line")
			continue
		}

		// Send to Warp
		if hasToken {
			err = warp.Send(datapoint.Encode())
			if err != nil {
				g.ReqTCPErrorCounter.Inc()
				log.WithFields(log.Fields{
					"error":  err,
					"txn":    txn,
					"metric": metric,
				}).Info("HTTP Post error")
				return
			}
			reqDp++
			log.Debug(datapoint)
		}
	}
}

// OpenWarp Get warp connection
func (g *Graphite) OpenWarp(token string, txn string) (*core.Warp, error) {
	// Get warp connection
	warp, err := core.NewWarp(token, txn, "")
	if err != nil {
		return nil, err
	}

	return warp, nil
}

func parseLine(metric string, parse bool) (*core.GTS, error) {
	split := strings.Split(metric, " ")

	// Expected format is : 'metrics value [timestamp]'
	if len(split) < 2 {
		return nil, errors.New("Bad metric format")
	}

	ts := time.Now().UnixNano() / 1000000

	if len(split) >= 3 {
		var err error
		ts, err = strconv.ParseInt(split[2], 10, 64)
		if err != nil {
			return nil, errors.New("Bad metric part: timestamp")
		}
	}

	var value interface{}
	skip := false

	// try to convert the string into a float64
	if strings.Contains(split[1], ".") {
		number, err := strconv.ParseFloat(split[1], 64)
		if err == nil {
			skip = true
			value = number
		}
	}

	// try to convert the string into an integer
	if !skip {
		number, err := strconv.ParseInt(split[1], 10, 64)
		if err == nil {
			skip = true
			value = number
		}
	}

	// try to convert the string into a boolean
	if !skip {
		if strings.ToLower(split[1]) == "true" {
			skip = true
			value = true
		} else if strings.ToLower(split[1]) == "false" {
			skip = true
			value = false
		}
	}

	// assume that the value is a string
	if !skip {
		value = split[1]
	}

	dp := &core.GTS{
		Ts:     float64(int64toTime(ts).UnixNano()) / 1000.0,
		Value:  value,
		Labels: make(map[string]string),
	}

	// Check if there are tags
	if strings.Contains(split[0], ";") {
		subSplit := strings.Split(split[0], ";")
		dp.Name = subSplit[0]

		// If no tags, but auto fill enabled, we map the hierarchy for later by label processing purpose
		if parse {
			classPart := strings.Split(subSplit[0], ".")
			for idx, part := range classPart {
				dp.Labels[strconv.Itoa(idx)] = part
			}
		}

		// Parse tags
		for _, v := range subSplit[1:] {
			tagSplit := strings.Split(v, "=")
			dp.Labels[tagSplit[0]] = tagSplit[1]
		}

	} else {
		dp.Name = split[0]

		// If no tags, but auto fill enabled, we map the hierarchy for later by label processing purpose
		if parse {
			classPart := strings.Split(split[0], ".")
			for idx, part := range classPart {
				dp.Labels[strconv.Itoa(idx)] = part
			}
		}
	}

	return dp, nil
}
