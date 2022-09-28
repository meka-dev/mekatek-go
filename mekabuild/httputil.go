package mekabuild

import (
	"compress/gzip"
	"fmt"
	"net/http"
	"strings"
)

// GunzipRequestMiddleware inspects the Content-Encoding header of the incoming
// request. If it specifies a supported compression scheme i.e. gzip, the body
// will be wrapped with a decompressor i.e. gzip.Reader.
func GunzipRequestMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("content-encoding"), "gzip") {
			zr, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, fmt.Errorf("gzip reader: %w", err).Error(), http.StatusBadRequest)
				return
			}
			r.Body = zr
		}
		h.ServeHTTP(w, r)
	})
}

// UserAgentDecorator sets the given User-Agent header on outgoing requests.
// It's intended to decorate the HTTP client provided to the builder.
func UserAgentDecorator(userAgent string) func(http.RoundTripper) http.RoundTripper {
	return func(rt http.RoundTripper) http.RoundTripper {
		return &userAgentDecorator{RoundTripper: rt, userAgent: userAgent}
	}
}

type userAgentDecorator struct {
	http.RoundTripper
	userAgent string
}

func (d *userAgentDecorator) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("user-agent", d.userAgent)
	return d.RoundTripper.RoundTrip(req)
}
