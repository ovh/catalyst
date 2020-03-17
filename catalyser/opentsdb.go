package catalyser

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ovh/catalyst/core"
)

// dataPoint represent a opentsdb point
type dataPoint struct {
	// The name of the metric you are storing
	Name string `json:"metric"`
	// A Unix epoch style timestamp in seconds or milliseconds
	Timestamp int64 `json:"timestamp"`
	// The value to record for this data point. Can be either an integer, a string or a float
	Value interface{} `json:"value"`
	// A map of tag name/tag value pairs. At least one pair must be supplied
	Tags map[string]string `json:"tags"`
}

// OpenTSDB returns an OpenTSDB catalyser.
func OpenTSDB(url *url.URL, header *http.Header, reader io.Reader, send func([]byte) error, dpCounter prometheus.Counter) (int, int, error) {
	dps := 0

	r := bufio.NewReader(reader)

	// check input format
	solo := true
	for {
		head, err := r.Peek(1)
		if err != nil {
			if err != io.EOF {
				return dps, -1, core.NewParsingError("Failed to parse datapoint - EOF", "")
			}
			return dps, -1, err
		}

		i := bytes.Index(head, []byte("["))
		j := bytes.Index(head, []byte("{"))
		if i < 0 && j < 0 {
			_, _ = r.Discard(1)
			continue
		} else {
			if j < 0 || (j < i) {
				solo = false
			}
			break
		}
	}

	// handle request
	dec := json.NewDecoder(r)

	if solo {
		gts, err := decode(dec)
		if err != nil {
			return dps, -1, err
		}

		err = send(gts.Encode())
		if err != nil {
			return dps, -1, err
		}

		dpCounter.Inc()
		dps++
	} else {
		// read open bracket
		_, err := dec.Token()
		if err != nil {
			return dps, -1, core.NewParsingError(fmt.Sprintf("Failed to parse datapoint: %v", err.Error()), "")
		}

		// while the array contains values
		for dec.More() {
			gts, err := decode(dec)
			if err != nil {
				return dps, -1, err
			}

			err = send(gts.Encode())
			if err != nil {
				return dps, -1, err
			}

			dpCounter.Inc()
			dps++
		}

		// read closing bracket
		_, err = dec.Token()
		if err != nil {
			return dps, -1, core.NewParsingError(fmt.Sprintf("Failed to parse datapoint: %v", err.Error()), "")
		}
	}

	return dps, http.StatusNoContent, nil
}

func decode(dec *json.Decoder) (*core.GTS, error) {
	var dp dataPoint
	err := dec.Decode(&dp)
	if err != nil {
		return nil, core.NewParsingError(fmt.Sprintf("Failed to parse datapoint: %v", err.Error()), "")
	}

	gts := core.GTS{
		Ts:     float64(int64toTime(dp.Timestamp).UnixNano() / 1000),
		Name:   dp.Name,
		Labels: dp.Tags,
		Value:  dp.Value,
	}

	return &gts, nil
}

// int64toTime Convert an int expressed either in seconds or milliseconds into a Time object
func int64toTime(timestamp int64) time.Time {
	if timestamp == 0 {
		return time.Now()
	}
	const nanosPerSec = 1000000000
	const nanosPerMilli = 1000000

	timeNanos := timestamp
	if timeNanos < 0xFFFFFFFF {
		// If less than 2^32, assume it's in seconds
		// (in millis that would be Thu Feb 19 18:02:47 CET 1970)
		timeNanos *= nanosPerSec
	} else {
		timeNanos *= nanosPerMilli
	}

	return time.Unix(timeNanos/nanosPerSec, timeNanos%nanosPerSec)
}
