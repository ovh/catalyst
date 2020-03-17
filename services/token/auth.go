package token

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	mutex sync.RWMutex

	bannished = map[string]struct{}{}

	bannishGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "catalyst",
		Subsystem: "bannish",
		Name:      "current",
		Help:      "Number of token in current session",
	})
)

func init() {
	prometheus.MustRegister(bannishGauge)
}

// Bannish a token
func Bannish(token string) {
	mutex.Lock()
	defer mutex.Unlock()
	bannishGauge.Inc()
	bannished[token] = struct{}{}
}

// IsBanned return true if the token is banned
func IsBanned(token string) bool {
	mutex.RLock()
	defer mutex.RUnlock()
	_, ok := bannished[token]
	return ok
}
