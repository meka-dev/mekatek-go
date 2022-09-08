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
