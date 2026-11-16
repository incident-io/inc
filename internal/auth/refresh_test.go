package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/incident-io/inc/internal/config"
)

// tokenServer is a fake OAuth token endpoint. respond writes the response to
// each refresh request; refreshes counts them.
type tokenServer struct {
	*httptest.Server
	refreshes atomic.Int32
}

func newTokenServer(t *testing.T, respond func(w http.ResponseWriter, refreshToken string)) *tokenServer {
	t.Helper()
	ts := &tokenServer{}
	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/plugin/token" {
			http.NotFound(w, r)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("token request body: %v", err)
		}
		if body["grant_type"] != "refresh_token" || body["client_id"] != OAuthClientID {
			t.Errorf("unexpected token request: %v", body)
		}
		ts.refreshes.Add(1)
		respond(w, body["refresh_token"])
	}))
	t.Cleanup(ts.Close)
	return ts
}

func respondWithNewTokens(w http.ResponseWriter, _ string) {
	// Long enough that concurrent callers queue on the lock rather than
	// finishing one after another.
	time.Sleep(100 * time.Millisecond)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token":  "new-access",
		"token_type":    "Bearer",
		"expires_in":    3600,
		"refresh_token": "new-refresh",
	})
}

// staleLogin saves a login whose access token expires within the refresh
// window to a temporary config file, and returns it as Load would.
func staleLogin(t *testing.T, appURL string) *config.Config {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &config.Config{
		OAuthToken:        "old-access",
		OAuthExpiresAt:    time.Now().Add(time.Minute).UTC().Format(time.RFC3339),
		OAuthRefreshToken: "old-refresh",
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.AppURL = appURL
	return cfg
}

func TestResolve_RefreshesExpiringOAuthToken(t *testing.T) {
	ts := newTokenServer(t, func(w http.ResponseWriter, refreshToken string) {
		if refreshToken != "old-refresh" {
			t.Errorf("expected the stored refresh token, got %q", refreshToken)
		}
		respondWithNewTokens(w, refreshToken)
	})
	cfg := staleLogin(t, ts.URL)

	key, _, err := Resolve(context.Background(), cfg, "", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if key != "new-access" {
		t.Errorf("expected the refreshed access token, got %q", key)
	}

	saved, err := config.LoadFile()
	if err != nil {
		t.Fatal(err)
	}
	if saved.OAuthToken != "new-access" || saved.OAuthRefreshToken != "new-refresh" {
		t.Errorf("refreshed tokens not saved: %+v", saved)
	}
	expiresAt, err := time.Parse(time.RFC3339, saved.OAuthExpiresAt)
	if err != nil || time.Until(expiresAt) < 55*time.Minute {
		t.Errorf("expected a new expiry about an hour out, got %q", saved.OAuthExpiresAt)
	}
}

func TestResolve_RefreshesExpiredOAuthToken(t *testing.T) {
	ts := newTokenServer(t, respondWithNewTokens)
	cfg := staleLogin(t, ts.URL)
	cfg.OAuthExpiresAt = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)

	key, _, err := Resolve(context.Background(), cfg, "", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if key != "new-access" {
		t.Errorf("expected the refreshed access token, got %q", key)
	}
}

func TestResolve_ConcurrentCallsRefreshOnce(t *testing.T) {
	// Every caller holds the same stale login, as parallel inc processes
	// would. Presenting the refresh token twice would revoke the login, so
	// exactly one of them may refresh and the rest must use its token.
	ts := newTokenServer(t, respondWithNewTokens)
	cfg := staleLogin(t, ts.URL)

	const callers = 8
	keys := make([]string, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			own := *cfg
			keys[i], _, errs[i] = Resolve(context.Background(), &own, "", "", "test")
		}()
	}
	wg.Wait()

	if n := ts.refreshes.Load(); n != 1 {
		t.Errorf("expected exactly one refresh, got %d", n)
	}
	for i := range callers {
		if errs[i] != nil {
			t.Errorf("caller %d: %v", i, errs[i])
		} else if keys[i] != "new-access" {
			t.Errorf("caller %d got %q, want the refreshed token", i, keys[i])
		}
	}
}

func TestResolve_InvalidGrantClearsLogin(t *testing.T) {
	ts := newTokenServer(t, func(w http.ResponseWriter, _ string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"The authorization code or refresh token is invalid, expired or revoked"}`))
	})
	cfg := staleLogin(t, ts.URL)

	_, _, err := Resolve(context.Background(), cfg, "", "", "test")
	if err == nil || !strings.Contains(err.Error(), "inc auth login") {
		t.Fatalf("expected an error pointing at 'inc auth login', got %v", err)
	}

	saved, err := config.LoadFile()
	if err != nil {
		t.Fatal(err)
	}
	if saved.OAuthToken != "" || saved.OAuthExpiresAt != "" || saved.OAuthRefreshToken != "" {
		t.Errorf("expected the dead login to be cleared, got %+v", saved)
	}
}

func TestResolve_OtherRefreshFailureKeepsLogin(t *testing.T) {
	// A 403 for the IP allowlist isn't invalid_grant: the refresh token still
	// works from an allowed network, so it must be kept.
	ts := newTokenServer(t, func(w http.ResponseWriter, _ string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"type":"forbidden","status":403,"errors":[{"code":"ip_not_allowed","message":"Request IP is not in the organisation's IP allowlist"}]}`))
	})
	cfg := staleLogin(t, ts.URL)

	if _, _, err := Resolve(context.Background(), cfg, "", "", "test"); err == nil {
		t.Fatal("expected the refresh failure to be returned")
	}

	saved, err := config.LoadFile()
	if err != nil {
		t.Fatal(err)
	}
	if saved.OAuthRefreshToken != "old-refresh" {
		t.Errorf("expected the refresh token to be kept, got %+v", saved)
	}
}
