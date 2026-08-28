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
		// A single object is a record, not a one-row list: with one value per
		// field, columns carry no information, and a show response has enough
		// fields that a horizontal layout truncates every one of them.
		return printRecord(w, fields, obj, opts)
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

// printRecord renders a single object vertically, one field per line. Fields
// select and order the lines the way they select and order a table's columns;
// without them, every top-level key appears, sorted.
func printRecord(w io.Writer, fields string, obj map[string]any, opts tableOpts) error {
	var keys []string
	if fields != "" {
		for _, f := range strings.Split(fields, ",") {
			keys = append(keys, strings.TrimSpace(f))
		}
	} else {
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}

	now := time.Now()
	labels := make([]string, len(keys))
	values := make([]string, len(keys))
	labelWidth, valueWidth := 0, 0
	for i, key := range keys {
		labelWidth = max(labelWidth, displayWidth(key))
		val := sanitizeCell(resolveField(obj, key))
		if opts.humanize && classifyColumn(key) == kindTimestamp {
			val = humanizeTime(val, now)
		}
		if opts.styled {
			val = paintCell(obj, key, val)
		}
		valueWidth = max(valueWidth, displayWidth(val))
		labels[i], values[i] = key, val
		if opts.styled {
			labels[i] = styleHeader.Sprint(key)
		}
	}

	// Labels always show in full: field names are short, and a truncated label
	// costs more than the value width it frees. The value takes what's left of
	// the terminal — unless that leaves it no room at all, where wrapping beats
	// a column of ellipses (the same call fitColumns makes for narrow tables).
	if avail := opts.maxWidth - labelWidth - len(columnGap); opts.maxWidth > 0 && avail >= minWidthForEllipsis {
		valueWidth = min(valueWidth, avail)
	}

	widths := []int{labelWidth, valueWidth}
	for i := range keys {
		if err := writeRow(w, []string{labels[i], values[i]}, widths); err != nil {
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

	return formatValue(columnLeaf(path), current)
}

// formatValue renders a decoded JSON value as a single cell — it avoids Go's
// default map printing. field is the name the value was read from: array
// elements consult it to tell a label from a value (see pairString).
func formatValue(field string, v any) string {
	if s, ok := scalarString(v); ok {
		return s
	}
	switch v := v.(type) {
	case nil:
		return ""
	case map[string]any:
		if label, ok := objectLabel(v); ok {
			return label
		}
		// A single-field object is a wrapper (the API's {value: ...} shape):
		// its content is the value.
		if len(v) == 1 {
			for key, inner := range v {
				return formatValue(key, inner)
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
		// Elements join when every one of them renders to something readable —
		// a scalar, a labelled object, or a definition/value pair. All or
		// nothing: joining just the readable subset would misreport the data,
		// so anything less falls back to a count.
		parts := make([]string, 0, len(v))
		for _, item := range v {
			part, ok := elementString(field, item)
			if !ok {
				parts = nil
				break
			}
			parts = append(parts, part)
		}
		if parts != nil {
			return strings.Join(parts, ", ")
		}
		if len(v) == 1 {
			return "[1 item]"
		}
		return fmt.Sprintf("[%d items]", len(v))
	default:
		return fmt.Sprintf("%v", v)
	}
}

// elementString renders one array element, reporting false for any element
// whose rendering would lose meaning, so the array falls back to a count.
func elementString(field string, v any) (string, bool) {
	if s, ok := scalarString(v); ok {
		return s, true
	}
	m, ok := v.(map[string]any)
	if !ok {
		return "", false
	}
	if label, ok := objectLabel(m); ok {
		return label, true
	}
	if len(m) == 1 {
		for key, inner := range m {
			return elementString(key, inner)
		}
	}
	return pairString(field, m)
}

// pairString renders the API's pervasive definition/value element: exactly two
// fields, one an object naming what the element is, the other its value for
// this record. A lone name-bearing object is the label; when both sides carry
// names (a role assignment: the role and the assignee are both named), the
// label is the side the array is named after — the API names these arrays
// after their definition type (role → incident_role_assignments, custom_field
// → custom_field_entries). Anything else is ambiguous and reports false.
func pairString(field string, m map[string]any) (string, bool) {
	if len(m) != 2 {
		return "", false
	}

	var labeled []string
	for key, val := range m {
		if obj, ok := val.(map[string]any); ok {
			if _, ok := obj["name"].(string); ok {
				labeled = append(labeled, key)
			}
		}
	}

	labelKey := ""
	switch len(labeled) {
	case 1:
		labelKey = labeled[0]
	case 2:
		for _, key := range labeled {
			if strings.Contains(field, key) {
				if labelKey != "" {
					return "", false
				}
				labelKey = key
			}
		}
		if labelKey == "" {
			return "", false
		}
	default:
		return "", false
	}

	label, _ := m[labelKey].(map[string]any)
	name, _ := label["name"].(string)
	for key, val := range m {
		if key != labelKey {
			// An empty value (an unset custom field) is just its name — a
			// dangling ": " would read as a value that failed to render.
			if value := formatValue(key, val); value != "" {
				return name + ": " + value, true
			}
			return name, true
		}
	}
	return "", false
}

// objectLabel picks a human-readable label from an object. Most incident.io
// objects have a "name" field.
func objectLabel(v map[string]any) (string, bool) {
	if name, ok := v["name"].(string); ok {
		return name, true
	}
	if email, ok := v["email"].(string); ok {
		return email, true
	}
	if value, ok := v["value"].(string); ok {
		return value, true
	}
	if id, ok := v["id"].(string); ok {
		return id, true
	}
	// Check for nested user object (e.g., creator.user.name)
	if user, ok := v["user"].(map[string]any); ok {
		if name, ok := user["name"].(string); ok {
			return name, true
		}
	}
	if apiKey, ok := v["api_key"].(map[string]any); ok {
		if name, ok := apiKey["name"].(string); ok {
			return name, true
		}
	}
	return "", false
}

// scalarString formats a JSON scalar, reporting false for anything that isn't
// one (objects, arrays, null).
func scalarString(v any) (string, bool) {
	switch s := v.(type) {
	case string:
		return s, true
	case float64:
		if s == float64(int64(s)) {
			return fmt.Sprintf("%d", int64(s)), true
		}
		return fmt.Sprintf("%g", s), true
	case bool:
		return fmt.Sprintf("%t", s), true
	}
	return "", false
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
