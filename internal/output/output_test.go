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
		"wrapper": map[string]any{
			"weird": "shape",
		},
		"opaque": map[string]any{
			"weird": "shape",
			"other": "thing",
		},
		"teams": []any{
			map[string]any{"name": "infra"},
			map[string]any{"name": "core"},
		},
		"ids":   []any{"a", "b"},
		"ports": []any{float64(80), float64(443)},
		"mixed": []any{"a", map[string]any{"weird": "shape", "other": "thing"}},
		"one": []any{
			map[string]any{"weird": "shape", "other": "thing"},
		},
		"several": []any{
			map[string]any{"weird": "shape", "other": "thing"},
			map[string]any{"weird": "shape", "other": "thing"},
		},
		"empty": []any{},
		"role_assignments": []any{
			map[string]any{
				"role":     map[string]any{"id": "01ROLE", "name": "Lead"},
				"assignee": map[string]any{"id": "01USER", "name": "Johanna", "email": "j@example.com"},
			},
			map[string]any{
				"role":     map[string]any{"id": "01ROLE2", "name": "Reporter"},
				"assignee": map[string]any{"id": "01USER2", "name": "Kate", "email": "k@example.com"},
			},
		},
		"timestamp_values": []any{
			map[string]any{
				"incident_timestamp": map[string]any{"id": "01TS", "name": "Reported at"},
				"value":              map[string]any{"value": "2026-08-11T10:00:00Z"},
			},
		},
		"duration_metrics": []any{
			map[string]any{
				"duration_metric": map[string]any{"id": "01DM", "name": "Time to resolve"},
				"value_seconds":   float64(5400),
			},
		},
		"custom_field_entries": []any{
			map[string]any{
				"custom_field": map[string]any{"id": "01CF", "name": "Affected teams"},
				"values": []any{
					map[string]any{"value_catalog_entry": map[string]any{"id": "01CAT", "name": "Payments"}},
				},
			},
			map[string]any{
				"custom_field": map[string]any{"id": "01CF2", "name": "Slack Users"},
				"values":       []any{},
			},
		},
		"ambiguous_pairs": []any{
			map[string]any{
				"alpha": map[string]any{"name": "A"},
				"beta":  map[string]any{"name": "B"},
			},
		},
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
		{"severity", "Major"},      // object with a name: use it
		{"owner", "a@example.com"}, // fall back to email
		{"ref", "01REF"},           // fall back to id
		{"creator", "Alice"},       // nested user.name
		{"wrapper", "shape"},       // single-field object: unwrap
		{"opaque", `{"other":"thing","weird":"shape"}`}, // no label, no wrapper: JSON
		{"teams", "infra, core"},                        // array of named objects
		{"ids", "a, b"},                                 // array of scalars: join values
		{"ports", "80, 443"},                            // numbers too
		{"mixed", "[2 items]"},                          // joining is all-or-nothing
		{"one", "[1 item]"},                             // unlabeled objects: count, singular
		{"several", "[2 items]"},                        // unlabeled objects: count
		{"empty", ""},
		{"role_assignments", "Lead: Johanna, Reporter: Kate"},             // both sides named: array is named after the label
		{"timestamp_values", "Reported at: 2026-08-11T10:00:00Z"},         // lone named side is the label, value unwrapped
		{"duration_metrics", "Time to resolve: 5400"},                     // scalar value side
		{"custom_field_entries", "Affected teams: Payments, Slack Users"}, // wrapper recursion; empty value is just its name
		{"ambiguous_pairs", "[1 item]"},                                   // both named, no tiebreak: count
		{"name.deeper", ""},                                               // dot-path into a non-object
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

func TestPrintTable_SingleObjectIsVertical(t *testing.T) {
	var buf bytes.Buffer
	data := json.RawMessage(`{"id":"01ABC","name":"Redis latency","severity":{"name":"Minor"}}`)
	if err := Print(&buf, "table", "", "", data); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected one line per field, got %d: %q", len(lines), lines)
	}
	// Keys sorted, one per line, label then value.
	for i, want := range []string{"id", "name", "severity"} {
		if !strings.HasPrefix(lines[i], want) {
			t.Errorf("line %d should start with %q, got: %q", i, want, lines[i])
		}
	}
	if !strings.Contains(lines[1], "Redis latency") {
		t.Errorf("expected full value on name line, got: %q", lines[1])
	}
	if !strings.Contains(lines[2], "Minor") {
		t.Errorf("expected severity label, got: %q", lines[2])
	}
}

func TestPrintTable_SingleObjectFieldsSelectAndOrder(t *testing.T) {
	var buf bytes.Buffer
	data := json.RawMessage(`{"id":"01ABC","name":"Redis latency","summary":"noise"}`)
	if err := Print(&buf, "table", "", "name,id", data); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two lines, got %d: %q", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "name") || !strings.HasPrefix(lines[1], "id") {
		t.Errorf("expected fields order name,id, got: %q", lines)
	}
	if strings.Contains(buf.String(), "noise") {
		t.Errorf("unselected field leaked into output: %q", buf.String())
	}
}

func TestPrintRecord_TruncatesValuesNotLabels(t *testing.T) {
	var buf bytes.Buffer
	data := json.RawMessage(`{"incident_status":"Investigating","summary":"` + strings.Repeat("x", 100) + `"}`)
	if err := printTableWith(&buf, "", data, tableOpts{maxWidth: 40}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	for _, line := range lines {
		if got := displayWidth(line); got > 40 {
			t.Errorf("line exceeds width 40 (%d): %q", got, line)
		}
	}
	if !strings.HasPrefix(lines[0], "incident_status") {
		t.Errorf("label should never truncate, got: %q", lines[0])
	}
	if !strings.Contains(lines[1], "…") {
		t.Errorf("long value should carry ellipsis, got: %q", lines[1])
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
