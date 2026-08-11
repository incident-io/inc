package output

import "strings"

// columnKind is what a column means, inferred from its name. It is the single
// place per-column policy lives: the width allocator asks what to protect, the
// palette asks what to colour. One classifier means those two can't drift into
// disagreeing about what a column is.
type columnKind int

const (
	// kindPlain is any column with no special handling.
	kindPlain columnKind = iota

	// kindHandle is a value you copy into the next command. Shortening one
	// makes it useless, so it keeps its width where it can.
	kindHandle

	// kindStatus is a lifecycle state, coloured by its category.
	kindStatus

	// kindSeverity is a severity, coloured by its rank.
	kindSeverity
)

// classifyColumn infers what a column holds from its name. Names are the only
// signal available: columns are dot-paths resolved out of decoded JSON, and
// every value arrives as a string, so `severity` and `title` are otherwise
// indistinguishable by the time the renderer sees them.
//
// Dot-paths classify on their root, so `severity` and `severity.name` behave
// the same way.
func classifyColumn(col string) columnKind {
	root := columnRoot(col)
	switch {
	case root == "id" || root == "reference" || strings.HasSuffix(root, "_id"):
		return kindHandle
	case root == "status" || strings.HasSuffix(root, "_status"):
		return kindStatus
	case root == "severity" || strings.HasSuffix(root, "_severity"):
		return kindSeverity
	default:
		return kindPlain
	}
}

// columnRoot returns the object a column reads from: the part before any dot.
func columnRoot(col string) string {
	root, _, _ := strings.Cut(col, ".")
	return root
}

// siblingValue reads another field from the object a column renders, so a
// colour can key off a stable value (a status's category, a severity's rank)
// rather than the label in the cell. Returns nil when the column isn't backed
// by an object.
func siblingValue(item map[string]any, col, field string) any {
	obj, ok := item[columnRoot(col)].(map[string]any)
	if !ok {
		return nil
	}
	return obj[field]
}

// sanitizeCell strips control characters from a value. Cell content comes from
// the API and is ultimately user-written, so it can carry escape sequences that
// would repaint or reposition the reader's terminal. Tabs and newlines become
// spaces to keep a row on one line; everything else in the C0 range goes.
func sanitizeCell(s string) string {
	// Printable ASCII is the overwhelming case and needs no rewriting. The
	// bail-out has to cover anything non-ASCII too, not just control bytes,
	// because strings.Map below also rewrites invalid UTF-8.
	clean := true
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 || c >= 0x7f {
			clean = false
			break
		}
	}
	if clean {
		return s
	}

	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return ' '
		case r < 0x20 || r == 0x7f:
			return -1
		case r >= 0x80 && r <= 0x9f:
			// C1 controls. U+009B is CSI, which terminals honouring 8-bit
			// controls treat exactly like ESC [, so leaving these in would
			// reopen the hole that stripping ESC closes.
			return -1
		}
		return r
	}, s)
}
