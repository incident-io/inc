package auth

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/incident-io/inc/internal/api"
	"github.com/incident-io/inc/internal/config"
)

// Resolve returns the API key and base URL using the precedence:
// flag > environment variable > config file.
//
// flagKey and flagURL are the values from --api-key and --api-url flags.
// Pass empty strings if the flags were not set.
func Resolve(cfg *config.Config, flagKey, flagURL string) (apiKey, apiURL string, err error) {
	// API key: flag > env > config (env already merged into cfg by viper)
	apiKey = cfg.APIKey
	if flagKey != "" {
		apiKey = flagKey
	}

	// API URL: flag > env > config > default
	apiURL = cfg.APIURL
	if apiURL == "" {
		apiURL = config.DefaultAPIURL
	}
	if flagURL != "" {
		apiURL = flagURL
	}

	if apiKey == "" {
		return "", "", api.NewUserError("no API key found. Set INCIDENT_API_KEY or run 'inc auth login'")
	}

	// Reject unusable URLs here so they surface as user errors instead of
	// masquerading as retryable network failures at request time.
	u, parseErr := url.Parse(apiURL)
	if parseErr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", api.NewUserError(fmt.Sprintf("invalid API URL %q. Expected something like %s", apiURL, config.DefaultAPIURL))
	}

	// The API key rides along as a bearer header, so plain HTTP would expose
	// it on the network. Allow http only for loopback (local development).
	if u.Scheme == "http" && !isLoopback(u.Hostname()) {
		return "", "", api.NewUserError(fmt.Sprintf("refusing to send the API key over plain HTTP to %q. Use https (http is allowed for localhost only)", u.Hostname()))
	}

	return apiKey, apiURL, nil
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
