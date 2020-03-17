package catalyser

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	log "github.com/sirupsen/logrus"

	"github.com/ovh/catalyst/core"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

// Prometheus returns a Prometheus catalyser.
func Prometheus(url *url.URL, headers *http.Header, r io.Reader, send func([]byte) error, dpCounter prometheus.Counter) (int, int, error) {
	dps := 0
	extraLabels := make(map[string]string)

	path := url.Path
	path = strings.TrimPrefix(path, "/prometheus")
	path = strings.TrimPrefix(path, "/metrics")
	path = strings.Trim(path, "/")

	pathLabels := strings.Split(path, "/")
	if len(pathLabels)%2 != 0 {
		return dps, -1, core.NewParsingError(fmt.Sprintf("Bad number of labels in URL (must be even but got : %d entries)", len(pathLabels)), path)
	}

	for i := 0; i < len(pathLabels); i = i + 2 {
		extraLabels[pathLabels[i]] = pathLabels[i+1]
	}

	format := expfmt.ResponseFormat(*headers)
	if format == expfmt.FmtUnknown {
		// Falling back to text mode if no format
		format = expfmt.FmtText
	}

	decoder := expfmt.NewDecoder(r, format)
	if decoder == nil {
		return dps, -1, core.NewParsingError("Unable to create decoder to decode response", path)
	}

	log.WithFields(log.Fields{
		"format": format,
	}).Println("Decoding Prometheus")

	for {
		// Decoding protobuff
		var mf dto.MetricFamily
		err := decoder.Decode(&mf)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.WithError(err).Errorln("Error decoding MetricFamily")
			return dps, -1, core.NewParsingError("Invalid format", path)
		}

		// Geting values from MetricFamily. We are injecting time.Now()
		// to force time if not set
		metrics, err := expfmt.ExtractSamples(&expfmt.DecodeOptions{
			Timestamp: model.TimeFromUnix(int64(time.Now().Unix())),
		}, &mf)
		if err != nil {
			log.WithError(err).Errorln("Error creating extractor")
			return dps, -1, core.NewParsingError("Invalid format", path)
		}

		// Creating GTS
		for _, metric := range metrics {
			dp := core.GTS{
				Labels: make(map[string]string),
			}

			if float64(metric.Value) == math.Inf(1) || float64(metric.Value) == math.Inf(-1) {
				continue
			}

			// get inner labels
			for key, value := range metric.Metric {
				if key == "__name__" {
					dp.Name = string(value)
					continue
				}
				dp.Labels[string(key)] = string(value)
			}

			// put additionnal labels
			for key, value := range extraLabels {
				dp.Labels[key] = value
			}
			// TS and value
			dp.Ts = float64(time.Unix(0, int64(metric.Timestamp)*1000*1000).UnixNano()) / 1000.0
			dp.Value = float64(metric.Value)

			// Send to Warp
			err = send(dp.Encode())
			if err != nil {
				return dps, -1, err
			}

			dpCounter.Inc()
			dps++

			log.Debug(string(dp.Encode()))
		}
	}
	return dps, http.StatusAccepted, nil
}
