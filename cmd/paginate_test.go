package cmd

import (
	"testing"
)

func TestExtractAfterCursor_WithCursor(t *testing.T) {
	body := `{"items":[],"pagination_meta":{"after":"abc123","page_size":25}}`
	cursor := extractAfterCursor([]byte(body))
	if cursor != "abc123" {
		t.Errorf("expected 'abc123', got '%s'", cursor)
	}
}

func TestExtractAfterCursor_NoCursor(t *testing.T) {
	body := `{"items":[],"pagination_meta":{"page_size":25}}`
	cursor := extractAfterCursor([]byte(body))
	if cursor != "" {
		t.Errorf("expected empty, got '%s'", cursor)
	}
}

func TestExtractAfterCursor_NoPaginationMeta(t *testing.T) {
	body := `{"items":[]}`
	cursor := extractAfterCursor([]byte(body))
	if cursor != "" {
		t.Errorf("expected empty, got '%s'", cursor)
	}
}

func TestExtractAfterCursor_EmptyCursor(t *testing.T) {
	body := `{"pagination_meta":{"after":""}}`
	cursor := extractAfterCursor([]byte(body))
	if cursor != "" {
		t.Errorf("expected empty, got '%s'", cursor)
	}
}

func TestExtractAfterCursor_InvalidJSON(t *testing.T) {
	cursor := extractAfterCursor([]byte(`not json`))
	if cursor != "" {
		t.Errorf("expected empty, got '%s'", cursor)
	}
}

func TestExtractAfterCursor_NullAfter(t *testing.T) {
	body := `{"pagination_meta":{"after":null}}`
	cursor := extractAfterCursor([]byte(body))
	if cursor != "" {
		t.Errorf("expected empty, got '%s'", cursor)
	}
}

func TestParseFields_GET(t *testing.T) {
	query, body := parseFields("GET", []string{"status[one_of]=live", "page_size=10"})
	if len(query) != 2 {
		t.Errorf("expected 2 query params, got %d", len(query))
	}
	if query["status[one_of]"][0] != "live" {
		t.Errorf("unexpected query param: %v", query)
	}
	if len(body) != 0 {
		t.Error("expected no body fields for GET")
	}
}

func TestParseFields_POST(t *testing.T) {
	query, body := parseFields("POST", []string{"name=test", "severity_id=01HXYZ"})
	if len(query) != 0 {
		t.Error("expected no query params for POST")
	}
	if body["name"] != "test" || body["severity_id"] != "01HXYZ" {
		t.Errorf("unexpected body: %v", body)
	}
}

func TestParseFields_MalformedSkipped(t *testing.T) {
	query, body := parseFields("GET", []string{"noequals", "good=value"})
	if len(query) != 1 {
		t.Errorf("expected 1 query param, got %d", len(query))
	}
	if len(body) != 0 {
		t.Errorf("expected 0 body fields, got %d", len(body))
	}
}
