package catalyser

import (
	"bufio"
	"io"
	"net/http"
	"net/url"
	"time"

	influxModel "github.com/influxdata/influxdb/models"
	"github.com/labstack/echo"
	"github.com/ovh/catalyst/core"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// InfluxDBVersion fixed INfluxDB supported version
	InfluxDBVersion          = "1.4.x"
	metricAndFieldsSeparator = "."
)

// InfluxDB returns an InfluxDB catalyser.
func InfluxDB(url *url.URL, header *http.Header, r io.Reader, send func([]byte) error, dpCounter prometheus.Counter) (int, int, error) {
	dps := 0
	precision := "n"
	if queryPrecision := url.Query().Get("precision"); queryPrecision != "" {
		precision = queryPrecision
	}

	// Get the stream in
	scan := bufio.NewScanner(r)
	var metric string
	for scan.Scan() {
		dp, err := parseInflux(scan.Bytes(), precision)
		if err != nil {
			return dps, -1, core.NewParsingError("Failed to parse datapoint", metric)
		}
		for _, point := range dp {
			err = send(point.Encode())
			if err != nil {
				return dps, -1, err
			}

			dpCounter.Inc()
			dps++
		}
	}

	return dps, http.StatusNoContent, nil
}

// HandlePing handle /ping call
func HandlePing(c echo.Context) error {
	c.Response().Header().Set("X-Influxdb-Version", InfluxDBVersion)
	c.Response().Header().Set("Request-Id", c.Get("txn").(string))
	return c.NoContent(http.StatusNoContent)
}

func parseInflux(in []byte, precision string) ([]core.GTS, error) {
	gts := []core.GTS{}
	// Use native InfluxDB parser
	points, err := influxModel.ParsePointsWithPrecision([]byte(in), time.Now(), precision)
	if err != nil {
		return nil, err
	}

	for _, point := range points {
		fields, err := point.Fields()
		if err != nil {
			return nil, err
		}

		for fieldName, fieldValue := range fields {
			gts = append(gts, core.GTS{
				Ts:     float64(point.Time().UnixNano() / 1e3),
				Name:   string(point.Name()) + metricAndFieldsSeparator + fieldName,
				Value:  fieldValue,
				Labels: point.Tags().Map(),
			})
		}
	}

	return gts, nil
}
