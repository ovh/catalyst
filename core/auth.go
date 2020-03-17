package core

import (
	"compress/gzip"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
)

// GetToken grep token from request
func GetToken(r *http.Request) (string, error) {
	t := r.Header.Get("X-Warp10-Token")
	if t != "" {
		return t, nil
	}

	t = r.Header.Get("X-Metrics-Token")
	if t != "" {
		return t, nil
	}

	t = r.Header.Get("X-CityzenData-Token")
	if t != "" {
		return t, nil
	}

	s := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
	if len(s) != 2 {
		return "", errors.New("missing basic auth bearer")
	}

	switch strings.ToLower(s[0]) {
	case "basic":
		b, err := base64.StdEncoding.DecodeString(s[1])
		if err != nil {
			return "", errors.New("bad basic auth bearer")
		}
		pair := strings.SplitN(string(b), ":", 2)
		if len(pair) != 2 {
			return "", errors.New("unauthorized")
		}

		return pair[1], nil
	case "bearer":
		return s[1], nil
	default:
		// retrieve token from influx db variables
		params := r.URL.Query()
		if t = params.Get("p"); t != "" {
			return t, nil
		}

		_ = r.ParseForm()
		if t = r.FormValue("p"); t != "" {
			return t, nil
		}

		return "", errors.New("invalid Authorization header")
	}
}

// handleGzip check if body is plain text or Gzip return a plain text reader
func handleGzip(r *http.Request) (io.Reader, error) {

	// If the request body is compressed, the Content-Type header MUST be set to the value application/gzip.
	if r.Header.Get("Content-Encoding") == "gzip" || r.Header.Get("Content-Type") == "application/gzip" {
		gReader, err := gzip.NewReader(r.Body)
		if err != nil {
			return nil, errors.New("failed to read gzip body")
		}
		return gReader, nil
	}
	return r.Body, nil
}
