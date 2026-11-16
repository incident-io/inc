package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/incident-io/inc/internal/api"
	"github.com/incident-io/inc/internal/config"
)

// refreshWindow is how close to its expiry an OAuth access token gets
// refreshed, so it doesn't expire partway through a command.
const refreshWindow = 5 * time.Minute

// Resolve returns the API key and base URL using the precedence:
// flag > environment variable > config file.
//
// flagKey and flagURL are the values from --api-key and --api-url flags.
// Pass empty strings if the flags were not set. When the credential is an
// OAuth access token close to expiry and there is a refresh token, Resolve
// refreshes it first; pluginVersion goes in that request's User-Agent.
func Resolve(ctx context.Context, cfg *config.Config, flagKey, flagURL, pluginVersion string) (apiKey, apiURL string, err error) {
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
		apiKey, err = oauthAccessToken(ctx, cfg, pluginVersion)
		if err != nil {
			return "", "", err
		}
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

func errLoginExpired() error {
	return api.NewUserError("your login has expired. Run 'inc auth login' to log in again")
}

// oauthAccessToken returns the stored OAuth access token, refreshing it first
// when it is close to expiry and there is a refresh token.
func oauthAccessToken(ctx context.Context, cfg *config.Config, pluginVersion string) (string, error) {
	expiresAt, err := time.Parse(time.RFC3339, cfg.OAuthExpiresAt)
	if err != nil {
		// A missing or corrupt expiry must not lock the user out: the token
		// is used as-is and the server decides whether it's still valid.
		return cfg.OAuthToken, nil
	}
	if time.Until(expiresAt) > refreshWindow {
		return cfg.OAuthToken, nil
	}
	if cfg.OAuthRefreshToken == "" {
		if time.Now().After(expiresAt) {
			return "", errLoginExpired()
		}
		return cfg.OAuthToken, nil
	}
	return refreshAccessToken(ctx, cfg.AppURL, pluginVersion)
}

// refreshAccessToken refreshes the stored OAuth login and returns the new
// access token.
//
// Each refresh token works once, and the server revokes the whole login if
// one is presented twice. inc often runs as several processes at once, so the
// refresh happens under the config lock, from the file as it is once the lock
// is held: if another process, or an earlier call in this one, has already
// refreshed, its token is used and this call doesn't refresh.
func refreshAccessToken(ctx context.Context, appURL, pluginVersion string) (string, error) {
	unlock, err := config.Lock(ctx)
	if err != nil {
		return "", err
	}
	defer unlock()

	cfg, err := config.LoadFile()
	if err != nil {
		return "", err
	}
	if cfg.OAuthToken != "" {
		if expiresAt, err := time.Parse(time.RFC3339, cfg.OAuthExpiresAt); err == nil && time.Until(expiresAt) > refreshWindow {
			return cfg.OAuthToken, nil
		}
	}
	if cfg.OAuthRefreshToken == "" {
		return "", errLoginExpired()
	}

	token, err := refreshOAuthToken(ctx, appURL, pluginVersion, cfg.OAuthRefreshToken)
	if errors.Is(err, errInvalidGrant) {
		// The login is gone for good (revoked, expired, or the refresh token
		// was already used). Clear it so later commands don't retry it.
		cfg.OAuthToken, cfg.OAuthExpiresAt, cfg.OAuthRefreshToken = "", "", ""
		if err := config.Save(cfg); err != nil {
			return "", fmt.Errorf("clearing expired login: %w", err)
		}
		return "", errLoginExpired()
	}
	if err != nil {
		return "", fmt.Errorf("refreshing login: %w", err)
	}

	cfg.OAuthToken = token.AccessToken
	cfg.OAuthExpiresAt = token.ExpiresAt.UTC().Format(time.RFC3339)
	cfg.OAuthRefreshToken = token.RefreshToken
	if err := config.Save(cfg); err != nil {
		// The old refresh token no longer works, so without the new one the
		// next refresh fails and the user has to log in again.
		return "", fmt.Errorf("saving refreshed login: %w", err)
	}
	return token.AccessToken, nil
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
