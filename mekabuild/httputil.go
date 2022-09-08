package mekabuild

import (
	"compress/gzip"
	"net/http"
	"strings"
)

func GunzipRequestMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("content-encoding"), "gzip") {
			if zr, err := gzip.NewReader(r.Body); err == nil {
				r.Body = zr
			}
		}
		h.ServeHTTP(w, r)
	})
}
