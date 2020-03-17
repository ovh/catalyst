package middlewares

import (
	"net/http"
	"time"

	"github.com/labstack/echo"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	"github.com/ovh/catalyst/core"
	tokenSrv "github.com/ovh/catalyst/services/token"
)

var (
	bannishCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "catalyst",
		Subsystem: "bannish",
		Name:      "request",
		Help:      "Number of request with this bannished token",
	}, []string{"token"})
)

func init() {
	prometheus.MustRegister(bannishCounter)
}

// Bannishment middleware respond a unauthorized status code if the token is
// banned. In addition, it wait the duration in order to preserve services
func Bannishment(duration time.Duration) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			token, err := core.GetToken(ctx.Request())
			if err != nil {
				log.WithError(err).WithFields(log.Fields{
					"txn": ctx.Get("txn"),
				}).Warn("Unauthorized: invalid credentials")
				return ctx.NoContent(http.StatusUnauthorized)
			}

			if tokenSrv.IsBanned(token) {
				log.WithFields(log.Fields{
					"txn": ctx.Get("txn"),
				}).Info("Unauthorized")
				bannishCounter.With(prometheus.Labels{"token": token}).Inc()
				time.Sleep(duration)
				return ctx.NoContent(http.StatusUnauthorized)
			}

			return next(ctx)
		}
	}
}
