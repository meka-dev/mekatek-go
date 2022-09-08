package mekabuild

import (
	"io"
	"net/http"
)

func GzipBodyReader(r *http.Request) io.ReadCloser {
	return nil
}
