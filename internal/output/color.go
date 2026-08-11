package output

import (
	"os"

	"github.com/fatih/color"
)

// style builds a palette entry that always emits its escapes. We decide whether
// to colour at all (see colorEnabled), and the library's own heuristic would
// otherwise override us: it drops colour whenever stdout isn't a terminal,
// which breaks CLICOLOR_FORCE and makes the styled path untestable through an
// io.Writer. Enabling per instance rather than resetting the library's global
// keeps that decision local — and the global reset didn't even work, since these
// vars are constructed before any init() could run.
func style(attrs ...color.Attribute) *color.Color {
	c := color.New(attrs...)
	c.EnableColor()
	return c
}

// Colours are the basic sixteen only. Terminals let users retheme those, so
// they adapt to light and dark backgrounds; a hardcoded 256-colour grey does
// not, which is what gh had to walk back for accessibility. Colour is never the
// only signal either: every cell reads the same with the escapes stripped.
var (
	styleHeader   = style(color.Bold)
	styleAttn     = style(color.FgRed)
	stylePending  = style(color.FgYellow)
	styleResolved = style(color.FgGreen)
)

// colorEnabled reports whether to emit colour, following the conventions the
// ecosystem settled on: NO_COLOR and CLICOLOR=0 turn it off, TERM=dumb turns it
// off, CLICOLOR_FORCE turns it on regardless, and otherwise it follows the
// terminal.
func colorEnabled() bool {
	if v := os.Getenv("CLICOLOR_FORCE"); v != "" && v != "0" {
		return true
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("CLICOLOR") == "0" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return IsTTY()
}

// statusStyle maps a status or status category to a colour. The vocabulary
// spans incidents (by category), alerts, escalations, follow-ups and
// post-mortems; anything unrecognised stays plain, including states that are
// over but not resolved, where colour would imply a judgement.
func statusStyle(value string) *color.Color {
	switch value {
	case "live", "firing", "triggered", "expired":
		return styleAttn
	case "triage", "learning", "acked", "pending", "pending_repeat", "delayed",
		"snoozed", "outstanding", "in_progress", "in_review":
		return stylePending
	case "closed", "resolved", "completed", "merged":
		return styleResolved
	}
	return nil
}

// severityStyle maps a severity's rank to a colour. Ranks are org-defined and
// open-ended, ordered least severe first, so the default three-severity setup
// (Minor, Major, Critical) lands on plain, yellow, red and anything above the
// second rank reads as most severe.
func severityStyle(rank int) *color.Color {
	switch {
	case rank >= 2:
		return styleAttn
	case rank == 1:
		return stylePending
	}
	return nil
}

// paintCell colours a rendered cell according to what its column holds. The
// colour keys off the underlying value rather than the label shown: a status's
// category and a severity's rank are stable, while the names beside them are
// org-customisable and unbounded.
func paintCell(item map[string]any, col, text string) string {
	if text == "" {
		return text
	}

	var style *color.Color
	switch classifyColumn(col) {
	case kindStatus:
		key, _ := siblingValue(item, col, "category").(string)
		if key == "" {
			// Alerts, escalations, follow-ups and post-mortems carry status as a
			// bare string rather than an object, and those strings are API
			// enums, so keying off the cell itself is still stable.
			key = text
		}
		style = statusStyle(key)
	case kindSeverity:
		if rank, ok := siblingValue(item, col, "rank").(float64); ok {
			style = severityStyle(int(rank))
		}
	}

	if style == nil {
		return text
	}
	return style.Sprint(text)
}
