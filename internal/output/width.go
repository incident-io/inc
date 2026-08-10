package output

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

const (
	// defaultWidth is used when the terminal size can't be read. Both gh and
	// docker settle on 80 for this.
	defaultWidth = 80

	// ellipsis marks a truncated cell. One cell wide, unlike "...".
	ellipsis = "…"

	// minWidthForEllipsis is the narrowest column that still gets a marker;
	// below it, cells are cut without one.
	minWidthForEllipsis = 4

	// columnGap separates columns. Budgeting and rendering both derive from it,
	// so they can't disagree about how much room the gaps take.
	columnGap = "  "

	// minFlexWidth is the narrowest a squeezed column may get before the
	// identifier column gives up its protection. An intact ID isn't worth
	// reducing every other column to initials.
	minFlexWidth = 8
)

// terminalWidth returns the width of stdout, or defaultWidth if it can't be
// read.
func terminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return defaultWidth
	}
	return w
}

// displayWidth measures a string as the terminal renders it: escape sequences
// count for nothing and wide runes count double. API values are overwhelmingly
// printable ASCII, so that case skips the parser entirely.
func displayWidth(s string) int {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 || c > 0x7e {
			return ansi.StringWidth(s)
		}
	}
	return len(s)
}

// naturalWidths returns the width each column needs to show its widest cell in
// full, header included.
func naturalWidths(cols []string, rows [][]string) []int {
	widths := make([]int, len(cols))
	for i, col := range cols {
		widths[i] = displayWidth(col)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				widths[i] = max(widths[i], displayWidth(cell))
			}
		}
	}
	return widths
}

// fitColumns returns the width to allow each column so a table of these columns
// and rows fits maxWidth.
//
// Columns keep their natural width when maxWidth is unknown (zero or less),
// when the table already fits, and when maxWidth is too small to give every
// column even one cell — wrapping beats a row of blank padding.
func fitColumns(cols []string, rows [][]string, maxWidth int) []int {
	natural := naturalWidths(cols, rows)

	budget := maxWidth - len(columnGap)*(len(cols)-1)
	total := 0
	for _, n := range natural {
		total += n
	}
	if maxWidth <= 0 || total <= budget || budget < len(cols) {
		return natural
	}

	// Only the row's own handle is worth protecting, and only while the other
	// columns stay readable. Treating every identifier as equally precious is
	// what makes protecting any of them unaffordable: a table carrying both an
	// id and an incident_id would lose both.
	handle := -1
	for i, col := range cols {
		if classifyColumn(col) == kindHandle {
			handle = i
			break
		}
	}
	if handle >= 0 {
		others := len(cols) - 1
		starved := others > 0 && (budget-natural[handle])/others < minFlexWidth
		if natural[handle] > budget || starved {
			handle = -1
		}
	}

	widths := make([]int, len(cols))
	flex := make([]int, 0, len(cols))
	remaining := budget
	for i := range cols {
		if i == handle {
			widths[i] = natural[i]
			remaining -= natural[i]
		} else {
			flex = append(flex, i)
		}
	}

	// Narrowest first: once a column can't afford an equal share of what's
	// left, no wider one can either, so those columns split the rest between
	// them. Settling the narrow columns first is what lets a wide column absorb
	// the slack.
	slices.SortStableFunc(flex, func(a, b int) int { return natural[a] - natural[b] })
	for k, i := range flex {
		rest := flex[k:]
		share, spare := remaining/len(rest), remaining%len(rest)
		if natural[i] > share {
			for n, j := range rest {
				widths[j] = share
				if n < spare {
					widths[j]++
				}
			}
			break
		}
		widths[i] = natural[i]
		remaining -= natural[i]
	}

	return widths
}

// truncateCell shortens a cell to width, appending an ellipsis when there's
// room for one. It is ANSI- and grapheme-aware, so styled and wide-rune content
// survives the cut.
func truncateCell(s string, width int) string {
	if width <= 0 {
		return ""
	}
	// ansi.Truncate does this too, but only after parsing the string; our
	// displayWidth answers from len() for the ASCII that cells almost always
	// are, so checking here skips the parser for every cell that already fits.
	if displayWidth(s) <= width {
		return s
	}
	tail := ""
	if width >= minWidthForEllipsis {
		tail = ellipsis
	}
	return ansi.Truncate(s, width, tail)
}

// padCell pads a cell to width, measuring what the terminal will show rather
// than counting runes or bytes, so wide runes and escape sequences don't shift
// the column.
func padCell(s string, width int) string {
	if pad := width - displayWidth(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

// writeRow renders one row: every cell fitted to its column, and padded to it
// except the last, which would only gain trailing whitespace.
func writeRow(w io.Writer, cells []string, widths []int) error {
	out := make([]string, len(cells))
	for i, cell := range cells {
		out[i] = truncateCell(cell, widths[i])
		if i < len(cells)-1 {
			out[i] = padCell(out[i], widths[i])
		}
	}
	_, err := fmt.Fprintln(w, strings.Join(out, columnGap))
	return err
}
