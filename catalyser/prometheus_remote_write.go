package catalyser

import (
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/ovh/catalyst/core"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/prompb"
	log "github.com/sirupsen/logrus"
)

// HandleRemoteWrite support remote_write protocol
// https://github.com/prometheus/prometheus/tree/0e0fc5a7f45ce28632f43f1ead0183ee82c7afca/documentation/examples/remote_storage/remote_storage_adapter
func HandleRemoteWrite(url *url.URL, headers *http.Header, r io.Reader, send func([]byte) error, dpCounter prometheus.Counter) (int, int, error) {
	dps := 0

	compressed, err := ioutil.ReadAll(r)
	if err != nil {
		log.WithError(err).Error("Cannot read body")
		return 0, http.StatusBadRequest, err
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		log.Error("msg", "Decode error", "err", err.Error())
		return 0, http.StatusInternalServerError, err
	}

	var wReq prompb.WriteRequest
	if err := proto.Unmarshal(reqBuf, &wReq); err != nil {
		return 0, http.StatusBadRequest, err
	}

	for _, promGts := range wReq.GetTimeseries() {
		for _, gts := range formatPromGts(promGts) {
			_ = send(gts.Encode())
			dpCounter.Inc()
			dps++
		}
	}

	// dps processed, response status code, error
	return dps, http.StatusOK, nil
}

func formatPromGts(ts *prompb.TimeSeries) []*core.GTS {
	gtss := make([]*core.GTS, len(ts.GetSamples()))

	name := ""
	labels := map[string]string{}

	for _, label := range ts.GetLabels() {
		if label.GetName() == "__name__" {
			name = label.GetValue()
			continue
		}

		labels[label.GetName()] = label.GetValue()
	}

	for i, dp := range ts.GetSamples() {
		v := dp.GetValue()

		// +Inf, -Inf -> 0
		if v == math.Inf(1) || v == math.Inf(-1) || math.IsNaN(v) {
			v = 0
		}

		gtss[i] = &core.GTS{
			Name:   name,
			Labels: labels,
			Ts:     float64(dp.GetTimestamp() * 1000), // ms -> μs
			Value:  v,
		}
	}

	log.Debug(len(gtss))

	return gtss
}
