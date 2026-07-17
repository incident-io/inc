package cmd

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
)

// parseErrorPayload decodes the structured JSON error printed on stderr.
func parseErrorPayload(t *testing.T, stderr string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(stderr), &payload); err != nil {
		t.Fatalf("stderr is not a JSON error payload: %v in %q", err, stderr)
	}
	return payload
}

func TestExitCode_APIError404(t *testing.T) {
	srv := newStubServer(t)
	body := `{"type":"not_found","status":404,"request_id":"req_show","errors":[{"code":"not_found","message":"Resource not found"}]}`
	srv.respondStatus("GET /v2/incidents/01NOPE", 404, body, nil)

	res := runCommand(t, srv.args("incidents", "show", "01NOPE")...)

	if res.exit != 2 {
		t.Fatalf("expected exit 2 for API error, got %d", res.exit)
	}
	payload := parseErrorPayload(t, res.stderr)
	if payload["message"] != "Resource not found" {
		t.Errorf("expected the API's message, got: %v", payload["message"])
	}
	if payload["retryable"] != false {
		t.Errorf("404 should not be retryable, got: %v", payload["retryable"])
	}
	if payload["request_id"] != "req_show" {
		t.Errorf("expected request_id passthrough, got: %v", payload["request_id"])
	}
	sent, _ := json.Marshal(payload["api_error"])
	var want, got map[string]any
	if err := json.Unmarshal([]byte(body), &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(sent, &got); err != nil {
		t.Fatalf("api_error is not an object: %v", err)
	}
	if got["request_id"] != want["request_id"] || got["type"] != want["type"] {
		t.Errorf("api_error must carry the API body verbatim, got: %s", sent)
	}
}

func TestExitCode_Sustained429RetriesThenExits2(t *testing.T) {
	srv := newStubServer(t)
	srv.respondStatus("GET /v2/users", 429,
		`{"message":"rate limited"}`, map[string]string{"Retry-After": "0"})

	res := runCommand(t, srv.args("users", "list")...)

	if res.exit != 2 {
		t.Fatalf("expected exit 2 after exhausted 429 retries, got %d, stderr: %s", res.exit, res.stderr)
	}
	if len(srv.requests) != 3 {
		t.Errorf("expected 3 attempts, got %d", len(srv.requests))
	}
	payload := parseErrorPayload(t, res.stderr)
	if payload["retryable"] != true {
		t.Errorf("429 should be retryable, got: %v", payload["retryable"])
	}
	if payload["error"] != "http_429" {
		t.Errorf("expected http_429, got: %v", payload["error"])
	}
}

func TestExitCode_NetworkError3(t *testing.T) {
	// Grab a port that nothing listens on.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadURL := "http://" + ln.Addr().String()
	_ = ln.Close()

	// A mutation: fails on the first attempt, so no retry backoff slows the test.
	res := runCommand(t, "incidents", "create", "--name", "t", "--visibility", "public",
		"--api-key", "test-key", "--api-url", deadURL)

	if res.exit != 3 {
		t.Fatalf("expected exit 3 for network error, got %d, stderr: %s", res.exit, res.stderr)
	}
	payload := parseErrorPayload(t, res.stderr)
	if payload["error"] != "network_error" || payload["retryable"] != true {
		t.Errorf("expected retryable network_error, got: %s", res.stderr)
	}
}

func TestExitCode_InvalidURLIsUserError(t *testing.T) {
	res := runCommand(t, "users", "list", "--api-key", "test-key", "--api-url", "api.incident.io")

	if res.exit != 1 {
		t.Fatalf("expected exit 1 for schemeless URL, got %d, stderr: %s", res.exit, res.stderr)
	}
	payload := parseErrorPayload(t, res.stderr)
	if msg, _ := payload["message"].(string); !strings.Contains(msg, "invalid API URL") {
		t.Errorf("expected invalid API URL message, got: %v", payload["message"])
	}
}

func TestExitCode_MissingAPIKeyIsUserError(t *testing.T) {
	res := runCommand(t, "users", "list")

	if res.exit != 1 {
		t.Fatalf("expected exit 1 without an API key, got %d, stderr: %s", res.exit, res.stderr)
	}
	payload := parseErrorPayload(t, res.stderr)
	if msg, _ := payload["message"].(string); !strings.Contains(msg, "no API key") {
		t.Errorf("expected missing key message, got: %v", payload["message"])
	}
}

func TestExitCode_UnknownFlagIsUserError(t *testing.T) {
	res := runCommand(t, "incidents", "list", "--no-such-flag")

	if res.exit != 1 {
		t.Fatalf("expected exit 1 for unknown flag, got %d", res.exit)
	}
	payload := parseErrorPayload(t, res.stderr)
	if msg, _ := payload["message"].(string); !strings.Contains(msg, "unknown flag") {
		t.Errorf("expected unknown flag message, got: %v", payload["message"])
	}
}
