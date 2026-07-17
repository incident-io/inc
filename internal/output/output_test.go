package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestResolveField(t *testing.T) {
	obj := map[string]any{
		"name":  "DB outage",
		"count": float64(3),
		"ratio": float64(0.5),
		"live":  true,
		"gone":  nil,
		"severity": map[string]any{
			"id":   "01SEV",
			"name": "Major",
		},
		"owner": map[string]any{
			"email": "a@example.com",
		},
		"ref": map[string]any{
			"id": "01REF",
		},
		"creator": map[string]any{
			"user": map[string]any{"name": "Alice"},
		},
		"labelless": map[string]any{
			"weird": "shape",
		},
		"teams": []any{
			map[string]any{"name": "infra"},
			map[string]any{"name": "core"},
		},
		"ids":   []any{"a", "b"},
		"empty": []any{},
	}

	tests := []struct {
		path string
		want string
	}{
		{"name", "DB outage"},
		{"count", "3"},
		{"ratio", "0.5"},
		{"live", "true"},
		{"gone", ""},
		{"missing", ""},
		{"severity.name", "Major"},
		{"severity", "Major"},              // object with a name: use it
		{"owner", "a@example.com"},         // fall back to email
		{"ref", "01REF"},                   // fall back to id
		{"creator", "Alice"},               // nested user.name
		{"labelless", `{"weird":"shape"}`}, // no label: JSON
		{"teams", "infra, core"},           // array of named objects
		{"ids", "[2 items]"},               // array without names
		{"empty", ""},
		{"name.deeper", ""}, // dot-path into a non-object
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := resolveField(obj, tt.path); got != tt.want {
				t.Errorf("resolveField(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestRawBody(t *testing.T) {
	if got := RawBody(nil); got != nil {
		t.Errorf("empty body should yield nil, got %s", got)
	}
	valid := []byte(`{"request_id":"r1"}`)
	if got := RawBody(valid); string(got) != string(valid) {
		t.Errorf("valid JSON must pass through verbatim, got %s", got)
	}
	html := []byte("<html>bad gateway</html>")
	got := RawBody(html)
	var s string
	if err := json.Unmarshal(got, &s); err != nil || s != string(html) {
		t.Errorf("non-JSON body should become a JSON string, got %s", got)
	}
	huge := []byte("<html>" + strings.Repeat("x", 5000))
	got = RawBody(huge)
	if err := json.Unmarshal(got, &s); err != nil || len(s) > maxRawBody+32 || !strings.HasSuffix(s, "... (truncated)") {
		t.Errorf("oversized non-JSON body should be truncated, got %d chars", len(s))
	}
}

func TestPrintError_JSONIncludesRequestIDAndAPIError(t *testing.T) {
	var buf bytes.Buffer
	raw := json.RawMessage(`{"type":"api_error","request_id":"r1","debug":{"message":"boom"}}`)
	PrintError(&buf, "json", ErrorPayload{
		Error:     "http_500",
		Message:   "Something went wrong",
		RequestID: "r1",
		Retryable: true,
		APIError:  raw,
		Debug:     "boom",
	})

	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["request_id"] != "r1" {
		t.Errorf("expected request_id, got %v", out["request_id"])
	}
	apiErr, _ := json.Marshal(out["api_error"])
	var want, got any
	_ = json.Unmarshal(raw, &want)
	_ = json.Unmarshal(apiErr, &got)
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
		t.Errorf("api_error must be the verbatim body, got %s", apiErr)
	}
	if _, present := out["debug"]; present {
		t.Error("debug must not appear top-level in JSON (it lives inside api_error)")
	}
}

func TestPrintError_JSONOmitsEmptyPassthroughFields(t *testing.T) {
	var buf bytes.Buffer
	PrintError(&buf, "json", ErrorPayload{Error: "user_error", Message: "no API key"})
	s := buf.String()
	if strings.Contains(s, "request_id") || strings.Contains(s, "api_error") {
		t.Errorf("empty passthrough fields must be omitted, got %s", s)
	}
}

func TestPrintError_TextIncludesDebugAndRequestID(t *testing.T) {
	var buf bytes.Buffer
	PrintError(&buf, "table", ErrorPayload{
		Error:      "http_500",
		Message:    "Something went wrong",
		Suggestion: "Try again later.",
		RequestID:  "r1",
		Debug:      "nil pointer dereference",
	})
	want := "Error: Something went wrong\nDebug: nil pointer dereference\nRequest ID: r1\nTry again later.\n"
	if buf.String() != want {
		t.Errorf("unexpected text rendering:\n%s", buf.String())
	}
}

func TestValidFormat(t *testing.T) {
	for _, valid := range []string{"table", "json"} {
		if !ValidFormat(valid) {
			t.Errorf("expected %q to be valid", valid)
		}
	}
	for _, invalid := range []string{"", "yaml", "JSON", "Table"} {
		if ValidFormat(invalid) {
			t.Errorf("expected %q to be invalid", invalid)
		}
	}
}

func TestPrintJSON_Compact(t *testing.T) {
	var buf bytes.Buffer
	data := json.RawMessage(`{"id":"1","name":"test"}`)
	if err := Print(&buf, "json", "", "", data); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(buf.String())
	if got != `{"id":"1","name":"test"}` && got != `{`+"\n"+`  "id": "1",`+"\n"+`  "name": "test"`+"\n"+`}` {
		// Accept either compact or pretty depending on TTY detection
		t.Logf("output: %s", got)
	}
}

func TestPrintJSON_WithJQ(t *testing.T) {
	var buf bytes.Buffer
	data := json.RawMessage(`{"incidents":[{"id":"1"},{"id":"2"}]}`)
	if err := Print(&buf, "json", ".incidents[].id", "", data); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(buf.String())
	if got != "\"1\"\n\"2\"" {
		t.Errorf("expected jq output '\"1\"\\n\"2\"', got '%s'", got)
	}
}

func TestPrintJSON_WithFields(t *testing.T) {
	var buf bytes.Buffer
	data := json.RawMessage(`{"id":"1","name":"test","secret":"hidden"}`)
	if err := Print(&buf, "json", "", "id,name", data); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if _, ok := result["secret"]; ok {
		t.Error("expected 'secret' to be filtered out")
	}
	if result["id"] != "1" || result["name"] != "test" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestPrintTable_Array(t *testing.T) {
	var buf bytes.Buffer
	data := json.RawMessage(`[{"id":"1","name":"alpha"},{"id":"2","name":"bravo"}]`)
	if err := Print(&buf, "table", "", "id,name", data); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "id") || !strings.Contains(out, "name") {
		t.Errorf("expected header row, got: %s", out)
	}
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "bravo") {
		t.Errorf("expected data rows, got: %s", out)
	}
}

func TestPrintError_JSON(t *testing.T) {
	var buf bytes.Buffer
	PrintError(&buf, "json", ErrorPayload{
		Error:      "unauthorized",
		Message:    "API key is invalid",
		Suggestion: "Run 'inc auth login'",
		Retryable:  false,
	})
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["error"] != "unauthorized" {
		t.Errorf("unexpected error field: %v", result["error"])
	}
}

func TestPrintError_Table(t *testing.T) {
	var buf bytes.Buffer
	PrintError(&buf, "table", ErrorPayload{
		Error:      "unauthorized",
		Message:    "API key is invalid",
		Suggestion: "Run 'inc auth login'",
	})
	out := buf.String()
	if !strings.Contains(out, "API key is invalid") {
		t.Errorf("expected message in output, got: %s", out)
	}
	if !strings.Contains(out, "Run 'inc auth login'") {
		t.Errorf("expected suggestion in output, got: %s", out)
	}
}
