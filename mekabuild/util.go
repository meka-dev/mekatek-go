package mekabuild

import (
	"net/url"
	"os"
	"strings"
)

// GetBuilderAPIURL returns a url.URL that points to the Mekatek builder API. If
// necessary, it can be overridden via the MEKATEK_BUILDER_API_URL environment
// variable.
func GetBuilderAPIURL() *url.URL {
	s := os.Getenv("MEKATEK_BUILDER_API_URL")
	if s == "" {
		return defaultBuilderAPIURL
	}

	if !strings.HasPrefix(s, "http") {
		s = defaultBuilderAPIURL.Scheme + "://" + s
	}

	u, err := url.Parse(s)
	if err != nil {
		return defaultBuilderAPIURL
	}

	return u
}

var defaultBuilderAPIURL = &url.URL{Scheme: "https", Host: "api.mekatek.xyz"}
