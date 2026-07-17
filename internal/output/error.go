package output

import (
	"encoding/json"
	"fmt"
	"io"
)

// ErrorPayload is the structured error format for JSON output. The top-level
// fields are the CLI's stable interface; APIError carries the API's response
// verbatim so nothing the server said is ever lost.
type ErrorPayload struct {
	Error      string          `json:"error"`
	Message    string          `json:"message"`
	Suggestion string          `json:"suggestion,omitempty"`
	RequestID  string          `json:"request_id,omitempty"`
	Retryable  bool            `json:"retryable"`
	APIError   json.RawMessage `json:"api_error,omitempty"`

	// Debug renders in the text format only; in JSON it already lives
	// inside APIError.
	Debug string `json:"-"`
}

// maxRawBody bounds non-JSON bodies (e.g. an HTML error page from a proxy)
// so error output stays readable.
const maxRawBody = 2048

// RawBody prepares a response body for embedding in an ErrorPayload: valid
// JSON passes through verbatim, anything else becomes a truncated JSON string.
func RawBody(body []byte) json.RawMessage {
	if len(body) == 0 {
		return nil
	}
	if json.Valid(body) {
		return json.RawMessage(body)
	}
	s := string(body)
	if len(s) > maxRawBody {
		s = s[:maxRawBody] + "... (truncated)"
	}
	quoted, err := json.Marshal(s)
	if err != nil {
		return nil
	}
	return quoted
}

// PrintError writes an error in the appropriate format.
// JSON format goes to the writer (usually stderr), plain text goes to the writer directly.
func PrintError(w io.Writer, format string, payload ErrorPayload) {
	if format == "json" {
		out, _ := json.Marshal(payload)
		_, _ = fmt.Fprintln(w, string(out))
		return
	}
	_, _ = fmt.Fprintf(w, "Error: %s\n", payload.Message)
	if payload.Debug != "" {
		_, _ = fmt.Fprintf(w, "Debug: %s\n", payload.Debug)
	}
	if payload.RequestID != "" {
		_, _ = fmt.Fprintf(w, "Request ID: %s\n", payload.RequestID)
	}
	if payload.Suggestion != "" {
		_, _ = fmt.Fprintf(w, "%s\n", payload.Suggestion)
	}
}
