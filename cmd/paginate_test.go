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

func TestResolveLimit(t *testing.T) {
	tests := []struct {
		name            string
		limit           int
		explicit        bool
		isTTY           bool
		want            int
		wantAutoLimited bool
	}{
		// The bug this exists to prevent: 0 is the documented way to ask for
		// everything, so passing it must not be read as "flag absent".
		{"explicit 0 on a terminal means everything", 0, true, true, 0, false},
		{"absent flag on a terminal gets our cap", 0, false, true, ttyDefaultLimit, true},
		{"explicit limit on a terminal is honoured", 5, true, true, 5, false},
		{"piped output is never capped", 0, false, false, 0, false},
		{"explicit limit when piped is honoured", 5, true, false, 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, autoLimited := resolveLimit(tt.limit, tt.explicit, tt.isTTY)
			if got != tt.want || autoLimited != tt.wantAutoLimited {
				t.Errorf("resolveLimit(%d, %v, %v) = (%d, %v), want (%d, %v)",
					tt.limit, tt.explicit, tt.isTTY, got, autoLimited, tt.want, tt.wantAutoLimited)
			}
		})
	}
}

func TestMoreResultsFollow(t *testing.T) {
	withCursor := []byte(`{"pagination_meta":{"after":"cursor-1"}}`)
	lastPage := []byte(`{"pagination_meta":{"page_size":25}}`)

	tests := []struct {
		name      string
		collected int
		limit     int
		body      []byte
		want      bool
	}{
		// The bug this exists to prevent: asking for exactly as many results as the
		// org has claimed there were more, so the notice told you to pass --limit 0
		// to see nothing extra.
		{"exact fit on the last page", 25, 25, lastPage, false},
		{"exact fit with another page to come", 25, 25, withCursor, true},
		{"rows dropped from the last page", 30, 25, lastPage, true},
		{"rows dropped and another page to come", 30, 25, withCursor, true},
		// An unreadable envelope can't promise more.
		{"unparseable body", 25, 25, []byte("not json"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := moreResultsFollow(tt.collected, tt.limit, tt.body); got != tt.want {
				t.Errorf("moreResultsFollow(%d, %d, %s) = %v, want %v",
					tt.collected, tt.limit, tt.body, got, tt.want)
			}
		})
	}
}
