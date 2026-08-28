package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/incident-io/inc/internal/api"
)

// OAuthClientID is the CLI's OAuth client, registered server-side with
// localhost-only redirect URIs.
const OAuthClientID = "incident-cli"

// oauthTimeout is how long we wait for the user to finish in the browser.
// Authorization codes expire server-side after 10 minutes anyway.
const oauthTimeout = 5 * time.Minute

// OAuthToken is the result of a completed browser login.
type OAuthToken struct {
	AccessToken string
	ExpiresAt   time.Time
}

// OAuthLogin runs the OAuth 2.0 authorization code + PKCE flow against the
// incident.io dashboard: it opens the consent page in the user's browser,
// receives the authorization code on a loopback listener, and exchanges it
// for an access token (currently 28 days, no refresh token).
//
// Progress messages for the terminal go to out.
func OAuthLogin(ctx context.Context, appURL, pluginVersion string, out io.Writer) (*OAuthToken, error) {
	if err := validateBaseURL(appURL, "app URL", "https://app.incident.io"); err != nil {
		return nil, err
	}

	verifier, challenge, err := newPKCE()
	if err != nil {
		return nil, err
	}
	state, err := randomToken(16)
	if err != nil {
		return nil, err
	}

	// Loopback listener on an ephemeral port. The server allows any localhost
	// port for the incident-cli client, so no port needs reserving.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to start local callback listener: %w", err)
	}
	defer ln.Close()
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port)

	type callback struct {
		code string
		err  error
	}
	resultCh := make(chan callback, 1)
	deliver := func(cb callback) {
		// Non-blocking: only the first callback counts, stray requests
		// (favicon probes, refreshes) must not hang.
		select {
		case resultCh <- cb:
		default:
		}
	}

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "State mismatch — please retry 'inc auth login'.", http.StatusBadRequest)
			deliver(callback{err: fmt.Errorf("state mismatch in OAuth callback")})
			return
		}
		if errCode := q.Get("error"); errCode != "" {
			writeCallbackPage(w, "Login failed", "Authorization was not granted. You can close this tab.")
			deliver(callback{err: fmt.Errorf("authorization failed: %s", errCode)})
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "Missing authorization code — please retry 'inc auth login'.", http.StatusBadRequest)
			deliver(callback{err: fmt.Errorf("OAuth callback carried no authorization code")})
			return
		}
		writeCallbackPage(w, "Logged in to the incident.io CLI", "You can close this tab and return to your terminal.")
		deliver(callback{code: code})
	})}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	deviceName, _ := os.Hostname()
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", OAuthClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("code_challenge", challenge)
	params.Set("code_challenge_method", "S256")
	params.Set("state", state)
	if deviceName != "" {
		params.Set("device_name", deviceName)
	}
	if pluginVersion != "" {
		params.Set("plugin_version", pluginVersion)
	}
	// The server responds with a 307 to the dashboard consent page, which
	// handles login if needed and redirects back to us with the code.
	authorizeURL := appURL + "/auth/plugin/authorize?" + params.Encode()

	fmt.Fprintf(out, "Opening your browser to log in to incident.io.\nIf it doesn't open, visit:\n\n  %s\n\n", authorizeURL)
	_ = openBrowser(authorizeURL) // best effort — the URL is printed above

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(oauthTimeout):
		return nil, fmt.Errorf("timed out after %s waiting for browser authorization", oauthTimeout)
	case cb := <-resultCh:
		if cb.err != nil {
			return nil, cb.err
		}
		return exchangeToken(ctx, appURL, pluginVersion, cb.code, verifier, redirectURI)
	}
}

// exchangeToken swaps the authorization code for an access token at the
// OAuth token endpoint.
func exchangeToken(ctx context.Context, appURL, pluginVersion, code, verifier, redirectURI string) (*OAuthToken, error) {
	body, err := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"code_verifier": verifier,
		"redirect_uri":  redirectURI,
		"client_id":     OAuthClientID,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, appURL+"/auth/plugin/token", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", api.UserAgent(pluginVersion))

	// The shared doer gives the exchange the same timeout and 429/5xx retry
	// behaviour as every other outbound call.
	resp, err := api.NewRetryDoer().Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading token exchange response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// Return the structured error so the root command renders exit code,
		// suggestion, and request ID as it does for every other API error.
		return nil, api.NewAPIErrorFromResponse(resp.StatusCode, respBody)
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil || parsed.AccessToken == "" {
		return nil, fmt.Errorf("token exchange returned an unexpected response")
	}

	return &OAuthToken{
		AccessToken: parsed.AccessToken,
		ExpiresAt:   time.Now().Add(time.Duration(parsed.ExpiresIn) * time.Second),
	}, nil
}

// newPKCE returns a code verifier and its S256 challenge.
func newPKCE() (verifier, challenge string, err error) {
	verifier, err = randomToken(32)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

func randomToken(byteLen int) (string, error) {
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate random value: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func writeCallbackPage(w http.ResponseWriter, title, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><html><head><title>%[1]s</title></head>
<body style="font-family: -apple-system, sans-serif; text-align: center; padding-top: 4rem;">
<h2>%[1]s</h2><p>%[2]s</p></body></html>`, title, message)
}

func openBrowser(u string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", u).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default:
		return exec.Command("xdg-open", u).Start()
	}
}
