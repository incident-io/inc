package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/mattn/go-isatty"
)

// IsTTY returns true if stdout is a terminal.
func IsTTY() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

// Formats lists the supported output formats.
var Formats = []string{"table", "json"}

// ValidFormat reports whether s is a supported output format.
func ValidFormat(s string) bool {
	return slices.Contains(Formats, s)
}

// Print writes data to stdout in the requested format.
//
//   - format: "json" or "table"
//   - jqExpr: optional JQ filter (only used with JSON output)
//   - fields: optional comma-separated field list
//   - data: the JSON data to output
func Print(w io.Writer, format, jqExpr, fields string, data json.RawMessage) error {
	switch format {
	case "json":
		return printJSON(w, jqExpr, fields, data)
	case "table":
		return printTable(w, fields, data)
	default:
		return fmt.Errorf("unknown output format: %s", format)
	}
}

func printJSON(w io.Writer, jqExpr, fields string, data json.RawMessage) error {
	// Apply field filtering first
	if fields != "" {
		var err error
		data, err = filterFields(data, strings.Split(fields, ","))
		if err != nil {
			return err
		}
	}

	// Apply JQ filter
	if jqExpr != "" {
		return applyJQ(w, jqExpr, data)
	}

	// Pretty-print if TTY, compact otherwise
	if IsTTY() {
		var buf bytes.Buffer
		if err := json.Indent(&buf, data, "", "  "); err != nil {
			return err
		}
		_, err := fmt.Fprintln(w, buf.String())
		return err
	}

	_, err := fmt.Fprintln(w, string(data))
	return err
}

// tableOpts is how a table is presented. Everything here is a terminal
// affordance: piped output takes the zero value, which keeps values whole,
// plain and machine-readable.
type tableOpts struct {
	// maxWidth constrains the table; zero or less means unconstrained.
	maxWidth int
	// styled colours cells.
	styled bool
	// humanize shows timestamps as an age rather than RFC3339.
	humanize bool
}

func printTable(w io.Writer, fields string, data json.RawMessage) error {
	// Width and humanized timestamps both follow the terminal, so piped output
	// stays complete and parseable. Colour is a separate decision: it consults
	// the terminal too, but CLICOLOR_FORCE deliberately overrides that, so the
	// two can't share one check.
	tty := IsTTY()
	opts := tableOpts{styled: colorEnabled(), humanize: tty}
	if tty {
		opts.maxWidth = terminalWidth()
	}
	return printTableWith(w, fields, data, opts)
}

// printTableWith renders a table with presentation explicitly supplied. Split
// out from printTable so tests can drive widths, styling and timestamps without
// owning a terminal.
func printTableWith(w io.Writer, fields string, data json.RawMessage, opts tableOpts) error {
	// Try to parse as an array of objects
	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		// Not an array — try single object
		var obj map[string]any
		if err := json.Unmarshal(data, &obj); err != nil {
			// Fall back to raw JSON. Sanitized like any other cell: a
			// non-conforming body could carry live escape sequences.
			_, e := fmt.Fprintln(w, sanitizeCell(string(data)))
			return e
		}
		items = []map[string]any{obj}
	}

	if len(items) == 0 {
		return nil
	}

	// Determine columns
	var cols []string
	if fields != "" {
		for _, f := range strings.Split(fields, ",") {
			cols = append(cols, strings.TrimSpace(f))
		}
	} else {
		// Use top-level keys from first item, sorted for deterministic output
		for k := range items[0] {
			cols = append(cols, k)
		}
		sort.Strings(cols)
	}

	now := time.Now()
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		vals := make([]string, len(cols))
		for i, col := range cols {
			// Sanitize before styling: values are user-controlled, so escapes
			// they carry must go before we add our own.
			vals[i] = sanitizeCell(resolveField(item, col))
			if opts.humanize && classifyColumn(col) == kindTimestamp {
				vals[i] = humanizeTime(vals[i], now)
			}
			if opts.styled {
				vals[i] = paintCell(item, col, vals[i])
			}
		}
		rows = append(rows, vals)
	}

	header := make([]string, len(cols))
	for i, col := range cols {
		header[i] = col
		if opts.styled {
			header[i] = styleHeader.Sprint(col)
		}
	}

	// Widths measure display width, so escapes don't change the layout and
	// truncation cuts around them. They do cost displayWidth's ASCII fast path,
	// which is affordable because colour is TTY-only, where the auto-limit keeps
	// tables small.
	widths := fitColumns(cols, rows, opts.maxWidth)
	if err := writeRow(w, header, widths); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeRow(w, row, widths); err != nil {
			return err
		}
	}
	return nil
}

// resolveField reads a value from a map, supporting dot-paths like "severity.name".
func resolveField(obj map[string]any, path string) string {
	parts := strings.Split(path, ".")
	var current any = obj

	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current, ok = m[part]
		if !ok {
			return ""
		}
	}

	// Format the final value — avoid Go's default map printing
	switch v := current.(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	case bool:
		return fmt.Sprintf("%t", v)
	case map[string]any:
		// Try to pick a human-readable label from the object.
		// Most incident.io objects have a "name" field.
		if name, ok := v["name"].(string); ok {
			return name
		}
		if email, ok := v["email"].(string); ok {
			return email
		}
		if id, ok := v["id"].(string); ok {
			return id
		}
		// Check for nested user object (e.g., creator.user.name)
		if user, ok := v["user"].(map[string]any); ok {
			if name, ok := user["name"].(string); ok {
				return name
			}
		}
		if apiKey, ok := v["api_key"].(map[string]any); ok {
			if name, ok := apiKey["name"].(string); ok {
				return name
			}
		}
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	case []any:
		if len(v) == 0 {
			return ""
		}
		// For arrays of objects, try to extract names
		names := make([]string, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if name, ok := m["name"].(string); ok {
					names = append(names, name)
					continue
				}
			}
		}
		if len(names) > 0 {
			return strings.Join(names, ", ")
		}
		return fmt.Sprintf("[%d items]", len(v))
	default:
		return fmt.Sprintf("%v", v)
	}
}

func applyJQ(w io.Writer, expr string, data json.RawMessage) error {
	query, err := gojq.Parse(expr)
	if err != nil {
		return fmt.Errorf("invalid jq expression: %w", err)
	}

	var input any
	if err := json.Unmarshal(data, &input); err != nil {
		return err
	}

	iter := query.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			return fmt.Errorf("jq error: %w", err)
		}
		out, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, string(out)); err != nil {
			return err
		}
	}
	return nil
}

// filterFields keeps only the specified keys in JSON objects.
// Works on both single objects and arrays of objects.
func filterFields(data json.RawMessage, fields []string) (json.RawMessage, error) {
	fieldSet := make(map[string]bool, len(fields))
	for _, f := range fields {
		fieldSet[strings.TrimSpace(f)] = true
	}

	// Try array
	var items []map[string]any
	if json.Unmarshal(data, &items) == nil {
		for i, item := range items {
			items[i] = pickFields(item, fieldSet)
		}
		return json.Marshal(items)
	}

	// Try single object
	var obj map[string]any
	if json.Unmarshal(data, &obj) == nil {
		return json.Marshal(pickFields(obj, fieldSet))
	}

	return data, nil
}

func pickFields(obj map[string]any, fields map[string]bool) map[string]any {
	result := make(map[string]any, len(fields))
	for k, v := range obj {
		if fields[k] {
			result[k] = v
		}
	}
	return result
}
