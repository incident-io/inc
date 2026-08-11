package output

import (
	"fmt"
	"time"
)

// absoluteAfter is where relative time stops being useful. Inside it "3d ago"
// tells you what you want; past it a date is more informative than a count of
// days, and it's the same width or narrower.
const absoluteAfter = 30 * 24 * time.Hour

// humanizeTime rewrites an RFC3339 timestamp as the age a reader cares about:
// "5m ago", "2h ago", "3d ago", and a plain date once it's older than a month.
// Values that aren't timestamps come back untouched, so a mislabelled column
// can't mangle its own contents.
//
// now is a parameter rather than time.Now() so the formatting is testable.
func humanizeTime(value string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}

	d := now.Sub(t)
	if d < 0 {
		// Schedules and escalations carry timestamps in the future.
		return "in " + shortDuration(-d)
	}
	if d >= absoluteAfter {
		return t.Local().Format(time.DateOnly)
	}
	if d < time.Minute {
		return "just now"
	}
	return shortDuration(d) + " ago"
}

// shortDuration renders a duration in its largest useful unit: 45s, 5m, 2h, 3d.
func shortDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
