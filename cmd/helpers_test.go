package cmd

import (
	"errors"
	"testing"

	"github.com/incident-io/inc/internal/api"
)

func TestHandleAPIResponse_Success(t *testing.T) {
	err := handleAPIResponse(200, []byte(`{"ok":true}`))
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestHandleAPIResponse_404(t *testing.T) {
	err := handleAPIResponse(404, []byte(`{"errors":[{"message":"not found"}]}`))
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		t.Fatal("expected APIError")
	}
	if apiErr.Message != "not found" {
		t.Errorf("expected 'not found', got '%s'", apiErr.Message)
	}
	if apiErr.Suggestion == "" {
		t.Error("expected suggestion for 404")
	}
}

func TestHandleAPIResponse_422MultipleErrors(t *testing.T) {
	body := `{"errors":[{"message":"field A required"},{"message":"field B invalid"}]}`
	err := handleAPIResponse(422, []byte(body))
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		t.Fatal("expected APIError")
	}
	if apiErr.Message != "field A required; field B invalid" {
		t.Errorf("unexpected message: '%s'", apiErr.Message)
	}
}

func TestResolveFormat(t *testing.T) {
	tests := []struct {
		name          string
		flagValue     string
		explicit      bool
		isTTY         bool
		configDefault string
		want          string
	}{
		{"explicit flag wins over pipe", "table", true, false, "json", "table"},
		{"explicit flag wins over config", "json", true, true, "table", "json"},
		{"piped defaults to json", "table", false, false, "table", "json"},
		{"config default applies on TTY", "table", false, true, "json", "json"},
		{"invalid or unreadable config falls back to flag default", "table", false, true, "", "table"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveFormat(tt.flagValue, tt.explicit, tt.isTTY, tt.configDefault)
			if got != tt.want {
				t.Errorf("resolveFormat(%q, %v, %v, %q) = %q, want %q",
					tt.flagValue, tt.explicit, tt.isTTY, tt.configDefault, got, tt.want)
			}
		})
	}
}

func TestWithDefaultFields(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		fields   string
		defaults string
		want     string
	}{
		{"applies on table when unset", "table", "", "id,name", "id,name"},
		{"explicit fields win", "table", "title", "id,name", "title"},
		{"json keeps every field", "json", "", "id,name", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := withDefaultFields(tt.format, tt.fields, tt.defaults)
			if got != tt.want {
				t.Errorf("withDefaultFields(%q, %q, %q) = %q, want %q",
					tt.format, tt.fields, tt.defaults, got, tt.want)
			}
		})
	}
}

func TestHandleAPIResponse_EmptyBody(t *testing.T) {
	err := handleAPIResponse(500, []byte(`{}`))
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		t.Fatal("expected APIError")
	}
	if apiErr.Message != "API returned 500" {
		t.Errorf("expected fallback message, got '%s'", apiErr.Message)
	}
	if !apiErr.Retryable {
		t.Error("expected 500 to be retryable")
	}
}
