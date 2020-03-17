package catalyser

import (
	"bufio"
	"io"
	"net/http"
	"net/url"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ovh/catalyst/core"
)

// Warp returns a Warp catalyser.
func Warp(url *url.URL, header *http.Header, r io.Reader, send func([]byte) error, dpCounter prometheus.Counter) (int, int, error) {
	dps := 0
	i := 0
	metrics := ""

	// handle request
	scan := bufio.NewScanner(r)
	for scan.Scan() {
		metric := scan.Text()

		metrics += metric + "\r\n"

		i++
		if i >= 27 {
			i = 0
			err := send([]byte(metrics))
			if err != nil {
				return dps, -1, err
			}
			metrics = ""
		}

		dpCounter.Inc()
		dps++
	}

	if i != 0 {
		err := send([]byte(metrics))
		if err != nil {
			return dps, -1, err
		}
	}

	return dps, http.StatusOK, nil
}

// WarpError handle errors.
func WarpError(err error) error {
	if err != nil {
		if ierr, ok := err.(core.WarpInputError); ok {
			return core.NewParsingError("Failed to parse datapoint", ierr.Str)
		}
		return err
	}

	return nil
}
