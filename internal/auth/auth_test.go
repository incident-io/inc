package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/incident-io/inc/internal/config"
)

func TestResolve_FlagOverridesAll(t *testing.T) {
	cfg := &config.Config{
		APIKey: "from-config",
		APIURL: "https://config.example.com",
	}
	key, url, err := Resolve(cfg, "from-flag", "https://flag.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if key != "from-flag" {
		t.Errorf("expected key 'from-flag', got '%s'", key)
	}
	if url != "https://flag.example.com" {
		t.Errorf("expected url 'https://flag.example.com', got '%s'", url)
	}
}

func TestResolve_FallsBackToConfig(t *testing.T) {
	cfg := &config.Config{
		APIKey: "from-config",
		APIURL: "https://config.example.com",
	}
	key, url, err := Resolve(cfg, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if key != "from-config" {
		t.Errorf("expected key 'from-config', got '%s'", key)
	}
	if url != "https://config.example.com" {
		t.Errorf("expected url 'https://config.example.com', got '%s'", url)
	}
}

func TestResolve_DefaultURL(t *testing.T) {
	cfg := &config.Config{
		APIKey: "some-key",
	}
	_, url, err := Resolve(cfg, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if url != config.DefaultAPIURL {
		t.Errorf("expected default URL '%s', got '%s'", config.DefaultAPIURL, url)
	}
}

func TestResolve_ErrorWhenNoKey(t *testing.T) {
	cfg := &config.Config{}
	_, _, err := Resolve(cfg, "", "")
	if err == nil {
		t.Fatal("expected error when no API key is set")
	}
}

func TestResolve_URLValidation(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"https anywhere", "https://api.staging.incident.io", false},
		{"http localhost", "http://localhost:8080", false},
		{"http loopback ip", "http://127.0.0.1:8399", false},
		{"http ipv6 loopback", "http://[::1]:8080", false},
		{"http non-loopback", "http://attacker.example", true},
		{"http private ip", "http://10.0.0.5", true},
		{"no scheme", "api.incident.io", true},
		{"bad scheme", "ftp://api.incident.io", true},
		{"empty host", "https://", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Resolve(&config.Config{APIKey: "key"}, "", tt.url)
			if tt.wantErr && err == nil {
				t.Errorf("expected %q to be rejected", tt.url)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected %q to be accepted, got %v", tt.url, err)
			}
		})
	}
}

func TestResolve_FallsBackToOAuthToken(t *testing.T) {
	cfg := &config.Config{
		OAuthToken:     "oauth-token",
		OAuthExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}
	key, _, err := Resolve(cfg, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if key != "oauth-token" {
		t.Errorf("expected key 'oauth-token', got '%s'", key)
	}
}

func TestResolve_APIKeyWinsOverOAuthToken(t *testing.T) {
	cfg := &config.Config{
		APIKey:         "from-config",
		OAuthToken:     "oauth-token",
		OAuthExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}
	key, _, err := Resolve(cfg, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if key != "from-config" {
		t.Errorf("expected key 'from-config', got '%s'", key)
	}
}

func TestResolve_ExpiredOAuthToken(t *testing.T) {
	cfg := &config.Config{
		OAuthToken:     "oauth-token",
		OAuthExpiresAt: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
	}
	_, _, err := Resolve(cfg, "", "")
	if err == nil {
		t.Fatal("expected error for expired OAuth token")
	}
	if !strings.Contains(err.Error(), "inc auth login") {
		t.Errorf("expected expiry error to point at 'inc auth login', got %v", err)
	}
}

func TestResolve_OAuthTokenWithUnparseableExpiry(t *testing.T) {
	// A missing or corrupt expiry must not lock the user out: the token is
	// used as-is and the server decides whether it's still valid.
	cfg := &config.Config{
		OAuthToken:     "oauth-token",
		OAuthExpiresAt: "not-a-timestamp",
	}
	key, _, err := Resolve(cfg, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if key != "oauth-token" {
		t.Errorf("expected key 'oauth-token', got '%s'", key)
	}
}
