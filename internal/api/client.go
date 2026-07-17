package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ExitCodeUserError is returned for bad flags, missing auth, etc.
const ExitCodeUserError = 1

// ExitCodeAPIError is returned for 4xx/5xx responses from the API.
const ExitCodeAPIError = 2

// ExitCodeNetworkError is returned for connection failures, timeouts, etc.
const ExitCodeNetworkError = 3

// ErrDryRun is returned after printing a dry-run request.
// Commands should treat this as a successful exit.
var ErrDryRun = fmt.Errorf("dry-run: request printed, not sent")

// APIError represents a structured error from the API or transport layer.
// For API-originated errors, Body carries the API's response verbatim: the
// CLI enriches errors (exit code, retryable, suggestion) but never reduces
// them, mirroring the lossless data path.
type APIError struct {
	StatusCode int
	ExitCode   int
	Err        string `json:"error"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
	Retryable  bool   `json:"retryable"`

	// Debug is the API's debug detail (dev environments), for text rendering.
	Debug string `json:"-"`
	// Body is the raw response body; the output layer renders it.
	Body []byte `json:"-"`
}

func (e *APIError) Error() string {
	return e.Message
}

// NewUserError creates an APIError for user-facing errors (bad input, missing config).
func NewUserError(message string) *APIError {
	return &APIError{
		ExitCode: ExitCodeUserError,
		Err:      "user_error",
		Message:  message,
	}
}

// NewAPIErrorFromResponse creates a structured error from an HTTP status code and body.
// Parses the incident.io error format: {"errors": [{"message": "..."}]} or {"message": "..."}.
func NewAPIErrorFromResponse(statusCode int, body []byte) *APIError {
	apiErr := &APIError{
		StatusCode: statusCode,
		ExitCode:   ExitCodeAPIError,
		Err:        fmt.Sprintf("http_%d", statusCode),
		Message:    fmt.Sprintf("API returned %d", statusCode),
		Retryable:  statusCode == 429 || statusCode >= 500,
	}

	switch statusCode {
	case 401:
		apiErr.Suggestion = "Run 'inc auth login' to set a new API key."
	case 403:
		apiErr.Suggestion = "Your API key may not have permission for this action."
	case 404:
		apiErr.Suggestion = "Check that the resource ID exists."
	}

	apiErr.Body = body

	var parsed struct {
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
		Errors    []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Debug *struct {
			Message string `json:"message"`
		} `json:"debug"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		if len(parsed.Errors) > 0 {
			msgs := make([]string, 0, len(parsed.Errors))
			for _, e := range parsed.Errors {
				if e.Message != "" {
					msgs = append(msgs, e.Message)
				}
			}
			if len(msgs) > 0 {
				apiErr.Message = strings.Join(msgs, "; ")
			}
		} else if parsed.Message != "" {
			apiErr.Message = parsed.Message
		}
		apiErr.RequestID = parsed.RequestID
		if parsed.Debug != nil {
			apiErr.Debug = parsed.Debug.Message
		}
	}

	return apiErr
}

// Client is a thin HTTP client for the incident.io API.
type Client struct {
	BaseURL    string
	APIKey     string
	UserAgent  string
	HTTPClient *http.Client
	MaxRetries int
	DryRun     bool
}

// UserAgent returns the User-Agent header value for this CLI version. Both
// the generic client and the typed SDK client send it.
func UserAgent(version string) string {
	return fmt.Sprintf("inc/%s", version)
}

// NewClient creates a new API client.
func NewClient(baseURL, apiKey, version string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     apiKey,
		UserAgent:  UserAgent(version),
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		MaxRetries: 3,
	}
}

// Do executes an HTTP request and returns the raw response body.
// Retries on 429 (rate limit) and transient network errors up to MaxRetries.
func (c *Client) Do(method, path string, query map[string][]string, body io.Reader) (json.RawMessage, error) {
	reqURL := c.BaseURL + path

	// Buffer the body so we can replay it on retries.
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, &APIError{
				ExitCode: ExitCodeUserError,
				Err:      "invalid_request",
				Message:  fmt.Sprintf("failed to read request body: %s", err),
			}
		}
	}

	if c.DryRun {
		printDryRunRequest(method, reqURL, query, bodyBytes)
		return nil, ErrDryRun
	}

	var bodyReader io.Reader
	if bodyBytes != nil {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(method, reqURL, bodyReader)
	if err != nil {
		return nil, &APIError{
			ExitCode: ExitCodeUserError,
			Err:      "invalid_request",
			Message:  fmt.Sprintf("failed to build request: %s", err),
		}
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Content-Type", "application/json")

	if len(query) > 0 {
		q := req.URL.Query()
		for k, vals := range query {
			for _, v := range vals {
				q.Add(k, v)
			}
		}
		req.URL.RawQuery = q.Encode()
	}

	doer := &RetryDoer{Client: c.HTTPClient, MaxRetries: c.MaxRetries}
	resp, err := doer.Do(req)
	if err != nil {
		return nil, &APIError{
			ExitCode:  ExitCodeNetworkError,
			Err:       "network_error",
			Message:   fmt.Sprintf("request failed: %s", err),
			Retryable: true,
		}
	}

	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &APIError{
			ExitCode:  ExitCodeNetworkError,
			Err:       "read_error",
			Message:   fmt.Sprintf("failed to read response: %s", err),
			Retryable: true,
		}
	}

	if resp.StatusCode >= 400 {
		return nil, NewAPIErrorFromResponse(resp.StatusCode, respBody)
	}

	return json.RawMessage(respBody), nil
}

// RetryDoer is an HTTP client that retries transient failures. 429 responses
// are retried for every method, honouring Retry-After, because a rate-limited
// request was never processed. Transport errors are only retried for GET and
// HEAD, where a replay cannot duplicate a mutation. Each attempt gets its own
// timeout from the wrapped client; sleeps between attempts don't count
// against it.
type RetryDoer struct {
	Client     *http.Client
	MaxRetries int
}

// NewRetryDoer returns a RetryDoer with the default timeout and retry count.
func NewRetryDoer() *RetryDoer {
	return &RetryDoer{
		Client:     &http.Client{Timeout: 30 * time.Second},
		MaxRetries: 3,
	}
}

func (d *RetryDoer) Do(req *http.Request) (*http.Response, error) {
	// A request whose body can't be rebuilt can't be safely retried.
	replayable := req.Body == nil || req.GetBody != nil

	var resp *http.Response
	var err error
	for attempt := range d.MaxRetries {
		attemptReq := req
		if attempt > 0 {
			attemptReq = req.Clone(req.Context())
			if req.GetBody != nil {
				body, bodyErr := req.GetBody()
				if bodyErr != nil {
					return nil, bodyErr
				}
				attemptReq.Body = body
			}
		}

		resp, err = d.Client.Do(attemptReq)
		last := attempt == d.MaxRetries-1

		if err != nil {
			idempotent := req.Method == http.MethodGet || req.Method == http.MethodHead
			if last || !idempotent || !replayable {
				return nil, err
			}
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests && !last && replayable {
			_ = resp.Body.Close()
			time.Sleep(retryAfter(resp, attempt))
			continue
		}

		return resp, nil
	}

	return resp, err
}

// DryRunTransport prints the request and returns ErrDryRun instead of sending it.
type DryRunTransport struct{}

func (t *DryRunTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	printDryRunRequest(req.Method, req.URL.String(), nil, body)

	// Return an error to stop the command from trying to parse a response.
	return nil, ErrDryRun
}

// printDryRunRequest prints an HTTP request as indented JSON to stderr. It is
// the single formatter behind --dry-run for both the generic and typed
// clients; headers are deliberately omitted so credentials never print.
func printDryRunRequest(method, url string, query map[string][]string, body []byte) {
	out := map[string]any{
		"method": method,
		"url":    url,
	}
	if len(query) > 0 {
		out["query"] = query
	}
	if len(body) > 0 {
		var parsed any
		if json.Unmarshal(body, &parsed) == nil {
			out["body"] = parsed
		} else {
			out["body"] = string(body)
		}
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(os.Stderr, string(data))
}

// retryAfter returns how long to wait before retrying, using the Retry-After
// header if present, otherwise exponential backoff.
func retryAfter(resp *http.Response, attempt int) time.Duration {
	if val := resp.Header.Get("Retry-After"); val != "" {
		if secs, err := strconv.Atoi(val); err == nil {
			return time.Duration(secs) * time.Second
		}
	}
	return time.Duration(1<<uint(attempt)) * time.Second
}
