package output

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"slices"
	"strings"
	"testing"
)

func TestDisplayWidth(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"plain ascii", "hello", 5},
		{"escape sequences are invisible", "\x1b[31mhello\x1b[0m", 5},
		{"wide runes count double", "日本語", 6},
		{"emoji cluster counts once", "🇬🇧", 2},
		{"empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayWidth(tt.in); got != tt.want {
				t.Errorf("displayWidth(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestTruncateCell(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		width int
		want  string
	}{
		{"fits untouched", "hello", 5, "hello"},
		{"truncates with ellipsis", "hello world", 8, "hello w…"},
		{"too narrow for a marker", "hello world", 3, "hel"},
		{"zero width", "hello", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateCell(tt.in, tt.width); got != tt.want {
				t.Errorf("truncateCell(%q, %d) = %q, want %q", tt.in, tt.width, got, tt.want)
			}
		})
	}
}

func TestTruncateCell_NeverExceedsWidth(t *testing.T) {
	// Cutting inside a wide rune or a styled span must still respect the
	// budget, otherwise columns misalign.
	for _, in := range []string{
		"日本語のテキストです",
		"\x1b[31mred text that is quite long\x1b[0m",
		"🇬🇧🇬🇧🇬🇧🇬🇧🇬🇧",
		"plain ascii text",
	} {
		for width := 1; width <= 12; width++ {
			got := truncateCell(in, width)
			if w := displayWidth(got); w > width {
				t.Errorf("truncateCell(%q, %d) rendered %d cells wide: %q", in, width, w, got)
			}
		}
	}
}

func TestTruncateCell_PreservesStyling(t *testing.T) {
	got := truncateCell("\x1b[31mhello world\x1b[0m", 8)
	if !strings.HasPrefix(got, "\x1b[31m") {
		t.Errorf("expected the opening escape to survive, got %q", got)
	}
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Errorf("expected a reset so color doesn't bleed, got %q", got)
	}
}

func TestPadCell_AlignsByDisplayWidth(t *testing.T) {
	// text/tabwriter measured cells in runes, which over-padded CJK and would
	// count ANSI escapes as visible. Padding must use display width instead.
	tests := []struct {
		name  string
		in    string
		width int
	}{
		{"ascii", "abc", 10},
		{"cjk", "日本語", 10},
		{"emoji", "🔥 fire", 10},
		{"styled", "\x1b[31mred\x1b[0m", 10},
		{"already wider than width", "abcdefghijkl", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := padCell(tt.in, tt.width)
			want := max(tt.width, displayWidth(tt.in))
			if w := displayWidth(got); w != want {
				t.Errorf("padCell(%q, %d) renders %d cells, want %d", tt.in, tt.width, w, want)
			}
		})
	}
}

func TestNaturalWidths_IncludesHeaders(t *testing.T) {
	cols := []string{"reference", "n"}
	rows := [][]string{{"INC-1", "a longer value"}}
	got := naturalWidths(cols, rows)
	want := []int{len("reference"), len("a longer value")}
	if !slices.Equal(got, want) {
		t.Errorf("naturalWidths = %v, want %v", got, want)
	}
}

func TestFitColumns_NaturalWidthsWhenItFits(t *testing.T) {
	cols := []string{"name", "status"}
	rows := [][]string{{"web", "firing"}}
	want := []int{len("name"), len("status")}
	if got := fitColumns(cols, rows, 80); !slices.Equal(got, want) {
		t.Errorf("fitColumns = %v, want natural %v", got, want)
	}
}

func TestFitColumns_UnknownWidthDoesNotConstrain(t *testing.T) {
	// maxWidth 0 is how piped output asks for full values.
	cols := []string{"name"}
	rows := [][]string{{"a very long value that must survive"}}
	want := []int{len("a very long value that must survive")}
	if got := fitColumns(cols, rows, 0); !slices.Equal(got, want) {
		t.Errorf("fitColumns = %v, want natural %v", got, want)
	}
}

func TestFitColumns_TooNarrowToFitFallsBackToNatural(t *testing.T) {
	// Below one cell per column, padding alone would overflow the terminal, so
	// wrapping real data beats printing blank padding.
	cols := []string{"title", "id", "status"}
	rows := [][]string{{"some title", "01K6B0H4Q7YMKKXP12HDB72BD2", "firing"}}
	want := naturalWidths(cols, rows)
	if got := fitColumns(cols, rows, 3); !slices.Equal(got, want) {
		t.Errorf("fitColumns = %v, want natural %v", got, want)
	}
}

func TestFitColumns_AllIdentifierColumnsStillFit(t *testing.T) {
	// Every column pinned means the pins must give way, or the row overflows.
	const ulid = "01K6B0H4Q7YMKKXP12HDB72BD2"
	cols := []string{"id", "incident_id"}
	rows := [][]string{{ulid, ulid}}

	widths := fitColumns(cols, rows, 40)
	total := len(columnGap) * (len(cols) - 1)
	for _, w := range widths {
		total += w
	}
	if total > 40 {
		t.Errorf("widths %v total %d cells, over the 40 budget", widths, total)
	}
}

func TestFitColumns_FitsBudget(t *testing.T) {
	cols := []string{"title", "description"}
	rows := [][]string{
		{strings.Repeat("a", 60), strings.Repeat("b", 60)},
	}
	widths := fitColumns(cols, rows, 40)
	total := len(columnGap) * (len(cols) - 1)
	for _, w := range widths {
		total += w
	}
	if total > 40 {
		t.Errorf("widths %v total %d cells, over the 40 budget", widths, total)
	}
}

func TestFitColumns_HandleColumnKeepsFullWidth(t *testing.T) {
	// The row's own handle survives whole — you copy it into the next command.
	// A secondary reference is expendable: protecting both is what made
	// protecting either unaffordable.
	const ulid = "01K6B0H4Q7YMKKXP12HDB72BD2"
	cols := []string{"id", "title", "incident_id"}
	rows := [][]string{{ulid, strings.Repeat("t", 90), ulid}}

	widths := fitColumns(cols, rows, 80)
	if widths[0] != len(ulid) {
		t.Errorf("id column = %d, want its natural %d", widths[0], len(ulid))
	}
	if widths[2] >= len(ulid) {
		t.Errorf("secondary incident_id should be squeezed, got %d", widths[2])
	}
	if widths[1] >= 90 {
		t.Errorf("title column should absorb the squeeze, got %d", widths[1])
	}
}

func TestFitColumns_ReferenceIsAHandle(t *testing.T) {
	// incidents lead with reference rather than id; it's the same kind of value.
	cols := []string{"reference", "name"}
	rows := [][]string{{"INC-22662", strings.Repeat("n", 100)}}

	widths := fitColumns(cols, rows, 40)
	if widths[0] != len("INC-22662") {
		t.Errorf("reference column = %d, want its natural %d", widths[0], len("INC-22662"))
	}
}

func TestFitColumns_IdentifierPinningYieldsWhenItStarvesOthers(t *testing.T) {
	// Two ULIDs would eat 52 of 80 cells, leaving the other five columns three
	// each. Readable data beats intact IDs, so the pins give way.
	const ulid = "01K6B0H4Q7YMKKXP12HDB72BD2"
	cols := []string{"id", "title", "description", "alert_source_id", "key", "status", "created_at"}
	rows := [][]string{{ulid, "Disk usage critical", "Disk exceeded 95%", ulid, "dedup-key", "firing", "2026-04-27T18:27:35.436Z"}}

	widths := fitColumns(cols, rows, 80)
	if widths[0] == len(ulid) {
		t.Error("id should have been squeezed rather than starve the other columns")
	}
}

func TestFitColumns_ShortColumnsKeepNaturalWidth(t *testing.T) {
	// status is far narrower than an equal share, so it should not be padded
	// down; the slack belongs to the wide column instead.
	cols := []string{"status", "title"}
	rows := [][]string{
		{"firing", strings.Repeat("t", 100)},
		{"resolved", strings.Repeat("t", 100)},
	}

	widths := fitColumns(cols, rows, 60)
	if widths[0] != len("resolved") {
		t.Errorf("status column = %d, want its natural %d", widths[0], len("resolved"))
	}
	if widths[1] <= 60/2 {
		t.Errorf("title should get the freed space, got %d", widths[1])
	}
}

func TestFitColumns_HeaderCountsTowardWidth(t *testing.T) {
	// A header longer than every value still needs room, or it gets cut.
	cols := []string{"new_incident_status", "x"}
	rows := [][]string{{"a", strings.Repeat("b", 100)}}

	widths := fitColumns(cols, rows, 60)
	if widths[0] != len("new_incident_status") {
		t.Errorf("column sized %d, want room for its %d-char header", widths[0], len("new_incident_status"))
	}
}

// Randomised check of the two invariants that matter: a fitted table never
// exceeds its budget, and a column with content is never rendered as nothing.
// Both bugs this caught (padding-only overflow at tiny widths, and all-identifier
// tables never relieving their pins) were invisible to the example-based tests.
func TestPrintTableWidth_NeverOverflows(t *testing.T) {
	// The one property that must hold for every shape: a rendered table fits the
	// width it was given. Asserted on the rendered lines rather than recomputed
	// from the renderer's own arithmetic, so a disagreement between the width
	// budget and the column gap shows up here.
	//
	// This found two bugs the example-based tests missed: padding-only overflow
	// at tiny widths, and all-identifier tables never relieving their pins.
	r := rand.New(rand.NewSource(7))
	values := []string{"", "v", "firing", "01K6B0H4Q7YMKKXP12HDB72BD2", "日本語のテキスト", "🔥 fire", strings.Repeat("v", 40)}
	names := []string{"id", "incident_id", "reference", "", "title", "status", "created_at"}

	for range 2000 {
		n := 1 + r.Intn(6)
		cols := make([]string, n)
		for i := range cols {
			cols[i] = names[r.Intn(len(names))]
		}
		items := make([]map[string]string, 1+r.Intn(3))
		for j := range items {
			item := make(map[string]string, n)
			for _, col := range cols {
				item[col] = values[r.Intn(len(values))]
			}
			items[j] = item
		}
		data, err := json.Marshal(items)
		if err != nil {
			t.Fatal(err)
		}
		width := 1 + r.Intn(120)

		var buf bytes.Buffer
		if err := printTableStyled(&buf, strings.Join(cols, ","), json.RawMessage(data), width, false); err != nil {
			t.Fatalf("cols=%v width=%d: %v", cols, width, err)
		}
		for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
			// Natural widths are expected when the width can't fit one cell per
			// column: wrapping beats blank padding.
			if w := displayWidth(line); w > width && width >= 3*n {
				t.Fatalf("cols=%v width=%d: line renders %d cells: %q", cols, width, w, line)
			}
		}
	}
}

func TestPrintTableWidth_FitsTheTerminal(t *testing.T) {
	// Covers the glue: fitting, truncating, padding and joining together. The
	// rendered line width is measured, not recomputed from the same constants
	// the renderer uses, so a disagreement between budget and gap would show up.
	data := json.RawMessage(`[
		{"id":"01K6B0H4Q7YMKKXP12HDB72BD2","title":"Disk usage critical on db-2","status":"firing"},
		{"id":"01KQ834QRFA2PZQ1PM31DQ0E2S","title":"日本語のインシデントタイトルです","status":"resolved"}
	]`)

	for _, width := range []int{20, 40, 60, 80, 120} {
		var buf bytes.Buffer
		if err := printTableStyled(&buf, "id,title,status", data, width, false); err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
			if w := displayWidth(line); w > width {
				t.Errorf("width %d: line renders %d cells: %q", width, w, line)
			}
		}
	}
}

func TestPrintTableWidth_UnconstrainedKeepsWholeValues(t *testing.T) {
	// How piped output asks for everything: nothing may be shortened.
	const title = "a title long enough that any terminal would want to shorten it"
	data := json.RawMessage(`[{"title":"` + title + `"}]`)

	var buf bytes.Buffer
	if err := printTableStyled(&buf, "title", data, 0, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), title) {
		t.Errorf("expected the full value, got %q", buf.String())
	}
}

func TestPrintTableWidth_ColumnsStayAligned(t *testing.T) {
	// Wide runes must not shift the columns that follow them.
	data := json.RawMessage(`[
		{"name":"ascii","status":"firing"},
		{"name":"日本語","status":"resolved"},
		{"name":"🔥 fire","status":"firing"}
	]`)

	var buf bytes.Buffer
	if err := printTableStyled(&buf, "name,status", data, 40, false); err != nil {
		t.Fatal(err)
	}
	// The final column must begin at the same cell on every row. Under
	// rune-counted padding the CJK row shifted it.
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	wantStart := -1
	for _, line := range lines {
		gap := strings.LastIndex(line, columnGap)
		if gap < 0 {
			t.Fatalf("no column gap in %q", line)
		}
		start := displayWidth(line[:gap+len(columnGap)])
		if wantStart < 0 {
			wantStart = start
			continue
		}
		if start != wantStart {
			t.Errorf("last column starts at cell %d, want %d: %q", start, wantStart, line)
		}
	}
}
