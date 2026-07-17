package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDo_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = fmt.Fprint(w, `{"ok": true}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-key", "test")
	resp, err := c.Do("GET", "/test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(resp) != `{"ok": true}` {
		t.Errorf("unexpected response: %s", resp)
	}
}

func TestDo_AuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "my-secret-key", "test")
	_, _ = c.Do("GET", "/test", nil, nil)

	if gotAuth != "Bearer my-secret-key" {
		t.Errorf("expected 'Bearer my-secret-key', got '%s'", gotAuth)
	}
}

func TestDo_QueryParams(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "key", "test")
	_, _ = c.Do("GET", "/test", map[string][]string{"foo": {"bar"}}, nil)

	if gotQuery != "foo=bar" {
		t.Errorf("expected 'foo=bar', got '%s'", gotQuery)
	}
}

func TestDo_PostBody(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "key", "test")
	_, _ = c.Do("POST", "/test", nil, strings.NewReader(`{"name":"test"}`))

	if gotBody != `{"name":"test"}` {
		t.Errorf("unexpected body: %s", gotBody)
	}
}

func TestDo_Retry429(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			_, _ = fmt.Fprint(w, `{"message":"rate limited"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"ok": true}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "key", "test")
	c.MaxRetries = 3
	resp, err := c.Do("GET", "/test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(resp) != `{"ok": true}` {
		t.Errorf("unexpected response: %s", resp)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestDo_Sustained429ReturnsRateLimitError(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(429)
		_, _ = fmt.Fprint(w, `{"message":"rate limited","request_id":"req_429"}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "key", "test")
	c.MaxRetries = 3
	_, err := c.Do("GET", "/test", nil, nil)

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %v", err)
	}
	if apiErr.RequestID != "req_429" {
		t.Errorf("expected request_id from the final 429, got '%s'", apiErr.RequestID)
	}
	if apiErr.StatusCode != 429 {
		t.Errorf("expected status 429, got %d", apiErr.StatusCode)
	}
	if apiErr.ExitCode != ExitCodeAPIError {
		t.Errorf("expected exit code %d, got %d", ExitCodeAPIError, apiErr.ExitCode)
	}
	if apiErr.Message != "rate limited" {
		t.Errorf("expected the API's rate limit message, got '%s'", apiErr.Message)
	}
	if !apiErr.Retryable {
		t.Error("expected 429 to be retryable")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

// failingTransport counts calls and always fails with a transport error.
type failingTransport struct{ calls int }

func (f *failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.calls++
	return nil, errors.New("connection refused")
}

func TestDo_NetworkErrorNotRetriedForMutations(t *testing.T) {
	ft := &failingTransport{}
	c := NewClient("http://example.invalid", "key", "test")
	c.HTTPClient = &http.Client{Transport: ft}
	c.MaxRetries = 3

	_, err := c.Do("POST", "/test", nil, strings.NewReader(`{"name":"test"}`))

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %v", err)
	}
	if apiErr.Err != "network_error" {
		t.Errorf("expected network_error, got '%s'", apiErr.Err)
	}
	if ft.calls != 1 {
		t.Errorf("expected exactly 1 attempt for POST, got %d", ft.calls)
	}
}

func TestDo_NetworkErrorRetriedForGet(t *testing.T) {
	ft := &failingTransport{}
	c := NewClient("http://example.invalid", "key", "test")
	c.HTTPClient = &http.Client{Transport: ft}
	c.MaxRetries = 2

	_, err := c.Do("GET", "/test", nil, nil)

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %v", err)
	}
	if ft.calls != 2 {
		t.Errorf("expected 2 attempts for GET, got %d", ft.calls)
	}
}

func TestDo_RetryPreservesBody(t *testing.T) {
	attempts := 0
	var lastBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		lastBody = string(buf[:n])
		if attempts < 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			return
		}
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "key", "test")
	c.MaxRetries = 3
	_, err := c.Do("POST", "/test", nil, strings.NewReader(`{"important":"data"}`))
	if err != nil {
		t.Fatal(err)
	}
	if lastBody != `{"important":"data"}` {
		t.Errorf("body not preserved on retry: got '%s'", lastBody)
	}
}

func TestDo_4xxReturnsAPIError(t *testing.T) {
	body := `{"type":"not_found","status":404,"request_id":"req_123","errors":[{"code":"not_found","message":"Resource not found"}],"debug":{"message":"record missing"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "key", "test")
	_, err := c.Do("GET", "/test", nil, nil)

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatal("expected APIError")
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", apiErr.StatusCode)
	}
	if apiErr.ExitCode != ExitCodeAPIError {
		t.Errorf("expected exit code %d, got %d", ExitCodeAPIError, apiErr.ExitCode)
	}
	if apiErr.Message != "Resource not found" {
		t.Errorf("expected 'Resource not found', got '%s'", apiErr.Message)
	}
	if apiErr.Suggestion != "Check that the resource ID exists." {
		t.Errorf("unexpected suggestion: '%s'", apiErr.Suggestion)
	}
	if apiErr.RequestID != "req_123" {
		t.Errorf("expected request_id to be lifted, got '%s'", apiErr.RequestID)
	}
	if apiErr.Debug != "record missing" {
		t.Errorf("expected debug message to be lifted, got '%s'", apiErr.Debug)
	}
	if string(apiErr.Body) != body {
		t.Errorf("expected the raw body to be preserved verbatim, got: %s", apiErr.Body)
	}
}

func TestDo_422ParsesErrorsArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		_, _ = fmt.Fprint(w, `{"type":"validation_error","status":422,"errors":[{"message":"Name is required"},{"message":"Severity must be set"}]}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "key", "test")
	_, err := c.Do("POST", "/test", nil, nil)

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatal("expected APIError")
	}
	if apiErr.Message != "Name is required; Severity must be set" {
		t.Errorf("unexpected message: '%s'", apiErr.Message)
	}
}

func TestDo_DryRun(t *testing.T) {
	c := NewClient("https://example.com", "key", "test")
	c.DryRun = true
	_, err := c.Do("POST", "/test", nil, strings.NewReader(`{"name":"test"}`))

	if !errors.Is(err, ErrDryRun) {
		t.Errorf("expected ErrDryRun, got %v", err)
	}
}

func TestNewAPIErrorFromResponse_401(t *testing.T) {
	err := NewAPIErrorFromResponse(401, []byte(`{"message":"unauthorized"}`))
	if err.Suggestion == "" {
		t.Error("expected suggestion for 401")
	}
	if err.Message != "unauthorized" {
		t.Errorf("expected 'unauthorized', got '%s'", err.Message)
	}
}

func TestNewAPIErrorFromResponse_403(t *testing.T) {
	err := NewAPIErrorFromResponse(403, []byte(`{}`))
	if err.Suggestion == "" {
		t.Error("expected suggestion for 403")
	}
}

func TestNewAPIErrorFromResponse_5xxRetryable(t *testing.T) {
	err := NewAPIErrorFromResponse(500, []byte(`{}`))
	if !err.Retryable {
		t.Error("expected 500 to be retryable")
	}
	err2 := NewAPIErrorFromResponse(422, []byte(`{}`))
	if err2.Retryable {
		t.Error("expected 422 to not be retryable")
	}
}

func TestNewUserError(t *testing.T) {
	err := NewUserError("bad input")
	if err.ExitCode != ExitCodeUserError {
		t.Errorf("expected exit code %d, got %d", ExitCodeUserError, err.ExitCode)
	}
	if err.Message != "bad input" {
		t.Errorf("expected 'bad input', got '%s'", err.Message)
	}
}
