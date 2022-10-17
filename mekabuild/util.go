package mekabuild

import (
	"net/url"
	"os"
	"strconv"
	"strings"
)

// DryRunMode returns true if the MEKATEK_BUILDER_API_DRY_RUN or
// ZENITH_DRY_RUN environment variable is set to true. This can control
// behavior in the Tendermint integration.
func DryRunMode() bool {
	for _, v := range []string{
		"ZENITH_DRY_RUN",
		"MEKATEK_BUILDER_API_DRY_RUN",
	} {
		if b, err := strconv.ParseBool(os.Getenv(v)); err == nil {
			return b
		}
	}
	return false
}

// GetBuilderAPIURL returns a url.URL that points to the Mekatek builder API. If
// necessary, it can be overridden via the MEKATEK_BUILDER_API_URL or ZENITH_API_URL
// environment variable.
func GetBuilderAPIURL() *url.URL {
	var s string
	for _, v := range []string{
		"ZENITH_API_URL",
		"MEKATEK_BUILDER_API_URL",
	} {
		if s = os.Getenv(v); s != "" {
			break
		}
	}

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
