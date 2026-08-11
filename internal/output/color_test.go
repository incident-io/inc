package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/fatih/color"
)

func TestColorEnabled(t *testing.T) {
	// IsTTY is false under test, so these cover everything except the terminal
	// check itself.
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"off when not a terminal", nil, false},
		{"CLICOLOR_FORCE overrides", map[string]string{"CLICOLOR_FORCE": "1"}, true},
		{"CLICOLOR_FORCE=0 is not a force", map[string]string{"CLICOLOR_FORCE": "0"}, false},
		// gh resolves this the same way: an explicit force wins over NO_COLOR,
		// since the user asking for colour is the more specific instruction.
		{"CLICOLOR_FORCE wins over NO_COLOR", map[string]string{"CLICOLOR_FORCE": "1", "NO_COLOR": "1"}, true},
		{"NO_COLOR alone", map[string]string{"NO_COLOR": "1"}, false},
		{"CLICOLOR=0", map[string]string{"CLICOLOR": "0"}, false},
		{"TERM=dumb", map[string]string{"TERM": "dumb"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range []string{"CLICOLOR_FORCE", "NO_COLOR", "CLICOLOR", "TERM"} {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if got := colorEnabled(); got != tt.want {
				t.Errorf("colorEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPaletteRendersDistinctly(t *testing.T) {
	// Asserting the styles differ in rendered output, not just that they're
	// different pointers: identity would hold even if every style were red.
	rendered := map[string]string{}
	for name, style := range map[string]*color.Color{
		"header":   styleHeader,
		"attn":     styleAttn,
		"pending":  stylePending,
		"resolved": styleResolved,
	} {
		out := style.Sprint("x")
		if !strings.Contains(out, "\x1b[") {
			t.Fatalf("%s emitted no escapes: %q — the palette must not depend on the library's own env heuristic", name, out)
		}
		if prev, clash := rendered[out]; clash {
			t.Errorf("%s and %s render identically as %q", name, prev, out)
		}
		rendered[out] = name
	}
}

func TestStatusStyle(t *testing.T) {
	// Values are the API's own vocabulary across incidents (categories), alerts,
	// escalations, follow-ups and post-mortems. Grouped by what a reader should
	// take from the colour rather than by the switch arms, so the test says
	// something the implementation doesn't.
	needsAttention := []string{"live", "firing", "triggered", "expired"}
	inProgress := []string{"triage", "learning", "acked", "pending", "snoozed", "outstanding", "in_progress", "in_review"}
	done := []string{"closed", "resolved", "completed", "merged"}
	// Over, but not resolved: colouring these would imply a judgement.
	neutral := []string{"canceled", "declined", "not_doing", "paused", "", "nonsense"}

	for _, v := range needsAttention {
		if statusStyle(v) != styleAttn {
			t.Errorf("%q should read as needing attention", v)
		}
	}
	for _, v := range inProgress {
		if statusStyle(v) != stylePending {
			t.Errorf("%q should read as in progress", v)
		}
	}
	for _, v := range done {
		if statusStyle(v) != styleResolved {
			t.Errorf("%q should read as done", v)
		}
	}
	for _, v := range neutral {
		if statusStyle(v) != nil {
			t.Errorf("%q should stay plain", v)
		}
	}
}

func TestSeverityStyle(t *testing.T) {
	// The default setup is Minor 0, Major 1, Critical 2; ranks are open-ended.
	if severityStyle(0) != nil {
		t.Error("lowest rank should stay plain")
	}
	if severityStyle(1) != stylePending {
		t.Error("middle rank should stand out mildly")
	}
	if severityStyle(2) != styleAttn || severityStyle(7) != styleAttn {
		t.Error("top ranks should read as most severe")
	}
}

func TestPaintCell_KeysOffCategoryNotName(t *testing.T) {
	// Status names are org-customisable, so the colour must come from the
	// category beside them.
	item := map[string]any{
		"incident_status": map[string]any{"name": "Firefighting", "category": "live"},
	}
	got := paintCell(item, "incident_status", "Firefighting")
	if got != styleAttn.Sprint("Firefighting") {
		t.Errorf("expected the category to drive the colour, got %q", got)
	}
}

func TestPaintCell_DotPathFindsTheSibling(t *testing.T) {
	item := map[string]any{
		"severity": map[string]any{"name": "Critical", "rank": float64(2)},
	}
	if got := paintCell(item, "severity.name", "Critical"); got != styleAttn.Sprint("Critical") {
		t.Errorf("expected rank to drive the colour through a dot-path, got %q", got)
	}
}

func TestPaintCell_PlainStatusStringsWork(t *testing.T) {
	// Alerts and follow-ups carry status as a bare string, not an object.
	item := map[string]any{"status": "firing"}
	if got := paintCell(item, "status", "firing"); got != styleAttn.Sprint("firing") {
		t.Errorf("expected a bare status string to colour, got %q", got)
	}
}

func TestPaintCell_LeavesOtherColumnsAlone(t *testing.T) {
	item := map[string]any{"title": "Disk usage critical", "id": "01ABC"}
	for col, text := range map[string]string{"title": "Disk usage critical", "id": "01ABC"} {
		if got := paintCell(item, col, text); got != text {
			t.Errorf("column %q should be plain, got %q", col, got)
		}
	}
}

func TestSanitizeCell(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text untouched", "Disk usage critical", "Disk usage critical"},
		{"escape byte stripped, leaving inert text", "safe\x1b[31mred\x1b[0m", "safe[31mred[0m"},
		// U+009B is CSI: on terminals honouring 8-bit controls it does the job of
		// ESC [ on its own, so stripping ESC alone would leave the hole open.
		{"C1 CSI stripped", "safe\u009b31mred", "safe31mred"},
		{"other C1 controls stripped", "a\u0080b\u009fc", "abc"},
		{"newlines become spaces", "line one\nline two", "line one line two"},
		{"tabs become spaces", "a\tb", "a b"},
		{"wide runes survive", "日本語 🔥", "日本語 🔥"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeCell(tt.in); got != tt.want {
				t.Errorf("sanitizeCell(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPrintTableStyled_ColorsAndStaysInWidth(t *testing.T) {
	data := json.RawMessage(`[
		{"reference":"INC-1","incident_status":{"name":"Triage","category":"triage"},"severity":{"name":"Critical","rank":2}},
		{"reference":"INC-2","incident_status":{"name":"Closed","category":"closed"},"severity":{"name":"Minor","rank":0}}
	]`)

	var buf bytes.Buffer
	if err := printTableWith(&buf, "reference,incident_status,severity", data, tableOpts{maxWidth: 60, styled: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.Contains(out, "\x1b[") {
		t.Fatal("expected escape sequences when styled")
	}
	// Colour must not change layout: escapes are invisible to width.
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if w := displayWidth(line); w > 60 {
			t.Errorf("line renders %d cells: %q", w, line)
		}
	}
	// The text still reads correctly with the styling removed, so colour is
	// never the only carrier of meaning.
	plain := ansi.Strip(out)
	for _, want := range []string{"reference", "INC-1", "Triage", "Critical", "Closed", "Minor"} {
		if !strings.Contains(plain, want) {
			t.Errorf("stripped output is missing %q:\n%s", want, plain)
		}
	}
}

func TestPrintTableStyled_UnstyledIsPlain(t *testing.T) {
	data := json.RawMessage(`[{"status":"firing","title":"Disk full"}]`)

	var buf bytes.Buffer
	if err := printTableWith(&buf, "status,title", data, tableOpts{maxWidth: 40}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("expected no escapes when unstyled, got %q", buf.String())
	}
}

func TestPrintTableStyled_TruncationKeepsColorBalanced(t *testing.T) {
	// A cut inside a styled span must still reset, or the colour bleeds into the
	// rest of the terminal.
	data := json.RawMessage(`[{"incident_status":{"name":"A very long status name indeed","category":"live"}}]`)

	var buf bytes.Buffer
	if err := printTableWith(&buf, "incident_status", data, tableOpts{maxWidth: 12, styled: true}); err != nil {
		t.Fatal(err)
	}
	out := strings.TrimRight(buf.String(), "\n")
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected the cell to be styled, got %q", out)
	}
	if !strings.HasSuffix(out, "\x1b[0m") {
		t.Errorf("expected a trailing reset after truncation, got %q", out)
	}
}

func TestClassifyColumn_ShippedColumnSets(t *testing.T) {
	// Every default column set the commands declare, so a new resource whose
	// column names don't match the classifier fails here instead of quietly
	// shipping grey. This is how `new_severity` slipped through review: the
	// status column beside it coloured, and nothing said the severity didn't.
	sets := map[string]string{
		"incidents":        "reference,name,incident_status,severity,created_at",
		"alerts":           "id,title,status,created_at",
		"users":            "id,name,email,role",
		"schedules":        "id,name,timezone,created_at",
		"escalations":      "id,title,status,priority,created_at",
		"escalation paths": "id,name,current_responders",
		"incident updates": "id,incident_id,new_incident_status,new_severity,created_at",
		"post-mortems":     "id,title,status,incident_id,created_at",
		"catalog entries":  "id,name,rank,created_at",
		"catalog types":    "id,name,type_name,created_at",
		"custom fields":    "id,name,field_type,created_at",
		"roles":            "id,name,shortform,role_type",
		"follow-ups":       "id,title,status,assignee,priority",
		"severities":       "id,name,rank",
	}

	// What each set should classify, by column name.
	want := map[string]columnKind{
		"id": kindHandle, "reference": kindHandle, "incident_id": kindHandle,
		"status": kindStatus, "incident_status": kindStatus, "new_incident_status": kindStatus,
		"severity": kindSeverity, "new_severity": kindSeverity,
		"created_at": kindTimestamp,
	}

	for resource, fields := range sets {
		for _, col := range strings.Split(fields, ",") {
			expected, special := want[col]
			if !special {
				expected = kindPlain
			}
			if got := classifyColumn(col); got != expected {
				t.Errorf("%s: column %q classified %v, want %v", resource, col, got, expected)
			}
		}
	}
}

func TestColorEnabled_ForcedIntoAPipe(t *testing.T) {
	// CLICOLOR_FORCE exists precisely to colour non-terminal output (piping to
	// less -R, for instance). Gating colour on IsTTY as well would silently
	// defeat it, so the width decision and the colour decision stay separate.
	for _, k := range []string{"NO_COLOR", "CLICOLOR", "TERM"} {
		t.Setenv(k, "")
	}
	t.Setenv("CLICOLOR_FORCE", "1")

	if !colorEnabled() {
		t.Fatal("CLICOLOR_FORCE must enable colour even though stdout is not a terminal under test")
	}
}
