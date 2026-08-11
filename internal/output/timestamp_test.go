package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestHumanizeTime(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"seconds ago reads as now", "2026-08-11T11:59:30Z", "just now"},
		{"minutes", "2026-08-11T11:45:00Z", "15m ago"},
		{"an hour rounds down", "2026-08-11T10:59:00Z", "1h ago"},
		{"hours", "2026-08-11T04:00:00Z", "8h ago"},
		{"days", "2026-08-08T12:00:00Z", "3d ago"},
		{"just inside the relative window", "2026-07-13T12:00:00Z", "29d ago"},
		{"older falls back to a date", "2026-06-12T08:54:43Z", "2026-06-12"},
		{"future", "2026-08-11T14:00:00Z", "in 2h"},
		// The API sends fractional seconds. Ages truncate rather than round, so
		// 14m59.6s reads as 14m: nothing is ever reported as older than it is.
		{"fractional seconds truncate", "2026-08-11T11:45:00.409Z", "14m ago"},
		{"offset timestamps parse", "2026-08-11T12:45:00+01:00", "15m ago"},
		{"not a timestamp is left alone", "Disk usage critical", "Disk usage critical"},
		{"empty stays empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := humanizeTime(tt.in, now); got != tt.want {
				t.Errorf("humanizeTime(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHumanizeTime_NeverWiderThanRFC3339(t *testing.T) {
	// The point of this is a narrower column, so every rendering must be shorter
	// than what it replaces.
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	const rfc3339Width = len("2026-04-27T18:27:35.436Z")

	for _, offset := range []time.Duration{
		0, 30 * time.Second, 5 * time.Minute, 90 * time.Minute,
		25 * time.Hour, 20 * 24 * time.Hour, 400 * 24 * time.Hour,
		-2 * time.Hour, -400 * 24 * time.Hour,
	} {
		in := now.Add(-offset).Format(time.RFC3339Nano)
		got := humanizeTime(in, now)
		if len(got) >= rfc3339Width {
			t.Errorf("offset %v rendered %q (%d chars), no narrower than RFC3339", offset, got, len(got))
		}
	}
}

func TestPrintTableWith_HumanizesOnlyWhenAsked(t *testing.T) {
	data := json.RawMessage(`[{"reference":"INC-1","created_at":"2020-01-01T00:00:00Z"}]`)

	var humanized bytes.Buffer
	if err := printTableWith(&humanized, "reference,created_at", data, tableOpts{maxWidth: 80, humanize: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(humanized.String(), "2020-01-01") || strings.Contains(humanized.String(), "T00:00:00Z") {
		t.Errorf("expected a date, got %q", humanized.String())
	}

	// Piped output keeps the timestamp the API sent, so scripts can parse it.
	var raw bytes.Buffer
	if err := printTableWith(&raw, "reference,created_at", data, tableOpts{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.String(), "2020-01-01T00:00:00Z") {
		t.Errorf("expected the raw timestamp, got %q", raw.String())
	}
}

func TestPrintTableWith_HumanizesNestedTimestamps(t *testing.T) {
	// Timestamps classify on the leaf, so a dot-path still gets humanized while
	// the object it reads from is classified by its root.
	data := json.RawMessage(`[{"schedule":{"name":"Primary","updated_at":"2019-05-05T00:00:00Z"}}]`)

	var buf bytes.Buffer
	if err := printTableWith(&buf, "schedule.updated_at", data, tableOpts{maxWidth: 80, humanize: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "2019-05-05") || strings.Contains(buf.String(), "T00:00:00Z") {
		t.Errorf("expected the nested timestamp humanized, got %q", buf.String())
	}
}
