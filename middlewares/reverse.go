package middlewares

import (
	"net/http"
	"strings"

	"github.com/labstack/echo"
	log "github.com/sirupsen/logrus"
)

// ReverseConfig is the configuration to describe
// a reverse proxy
type ReverseConfig struct {
	URL  string
	Path string
}

func reverse(config ReverseConfig, ctx echo.Context) error {
	req := ctx.Request()

	uri := config.URL + "/" + ctx.Param("*")
	if config.Path != "" {
		uri = config.URL + config.Path
	}
	if strings.Contains(req.RequestURI, "?") {
		interogation := strings.Index(req.RequestURI, "?")
		uri += req.RequestURI[interogation:]
	}

	log.WithFields(log.Fields{
		"reverse": uri,
		"remote":  ctx.RealIP(),
		"host":    req.Host,
		"uri":     req.RequestURI,
		"method":  req.Method,
		"path":    req.URL.Path,
		"referer": req.Referer(),
	}).Debug("Execute reverse proxy")

	req, err := http.NewRequest(ctx.Request().Method, uri, ctx.Request().Body)
	if err != nil {
		return ctx.String(http.StatusInternalServerError, err.Error())
	}

	req.Header = ctx.Request().Header
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.WithError(err).Error("Cannot execute the request on the endpoint")
		return ctx.NoContent(http.StatusBadGateway)
	}

	for k, v := range res.Header {
		if strings.HasPrefix(k, "X-Warp") {
			ctx.Response().Header().Set(k, v[0])
		}
	}

	return ctx.Stream(res.StatusCode, res.Header.Get("Content-Type"), res.Body)
}

// ReverseWithConfig execute a reverse proxy using
// the configuration given in parameters
func ReverseWithConfig(config ReverseConfig) echo.HandlerFunc {
	return func(ctx echo.Context) error {
		return reverse(config, ctx)
	}
}
