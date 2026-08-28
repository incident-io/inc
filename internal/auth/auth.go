package auth

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

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

	// Fall back to a browser-login (OAuth) token when no API key is set. The
	// token is a bearer credential exactly like an API key, so downstream
	// clients don't distinguish the two.
	if apiKey == "" && cfg.OAuthToken != "" {
		if expiresAt, err := time.Parse(time.RFC3339, cfg.OAuthExpiresAt); err == nil && time.Now().After(expiresAt) {
			return "", "", api.NewUserError("your login has expired. Run 'inc auth login' to log in again")
		}
		apiKey = cfg.OAuthToken
	}

	if apiKey == "" {
		return "", "", api.NewUserError("no API key found. Set INCIDENT_API_KEY or run 'inc auth login'")
	}

	// Reject unusable URLs here so they surface as user errors instead of
	// masquerading as retryable network failures at request time.
	if err := validateBaseURL(apiURL, "API URL", config.DefaultAPIURL); err != nil {
		return "", "", err
	}

	return apiKey, apiURL, nil
}

// validateBaseURL rejects URLs a bearer credential must not ride over: a
// missing or non-http(s) scheme, an empty host, or plain HTTP to anything
// but loopback (allowed for local development only). This is the transport
// rule for every credential-carrying URL — the API URL and the OAuth app
// URL both validate through here.
func validateBaseURL(rawURL, label, example string) error {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return api.NewUserError(fmt.Sprintf("invalid %s %q. Expected something like %s", label, rawURL, example))
	}
	if u.Scheme == "http" && !isLoopback(u.Hostname()) {
		return api.NewUserError(fmt.Sprintf("refusing to send credentials over plain HTTP to %q. Use https (http is allowed for localhost only)", u.Hostname()))
	}
	return nil
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
