package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/incident-io/inc/internal/output"
)

// PageFetcher fetches one page of results. The caller passes the "after"
// cursor (nil for the first page). Returns the raw response body and status code.
type PageFetcher func(after *string) (body []byte, statusCode int, err error)

// PaginateOpts configures the paginate helper.
type PaginateOpts struct {
	// DefaultFields are used for table output when the user doesn't pass --fields.
	DefaultFields string
}

// paginate fetches all pages, unwraps each envelope, collects items, and prints.
func paginate(cmd *cobra.Command, envelopeKey string, fetch PageFetcher) error {
	return paginateWith(cmd, envelopeKey, fetch, PaginateOpts{})
}

// paginateWith is like paginate but accepts options for default table fields.
func paginateWith(cmd *cobra.Command, envelopeKey string, fetch PageFetcher, opts PaginateOpts) error {
	format, jqExpr, fields := getOutputFlags(cmd)
	limit, _ := cmd.Flags().GetInt("limit")

	// Apply default table fields if user didn't specify --fields and we're in table mode
	if fields == "" && format == "table" && opts.DefaultFields != "" {
		fields = opts.DefaultFields
	}

	// Default to 30 results in TTY mode to avoid fetching everything
	autoLimited := false
	if limit == 0 && output.IsTTY() {
		limit = 30
		autoLimited = true
	}

	all := make([]json.RawMessage, 0)
	var after *string
	truncated := false

	for {
		body, status, err := fetch(after)
		if err != nil {
			return err
		}
		if err := handleAPIResponse(status, body); err != nil {
			return err
		}

		items, err := output.UnwrapEnvelope(body, envelopeKey)
		if err != nil {
			return err
		}

		var page []json.RawMessage
		if err := json.Unmarshal(items, &page); err != nil {
			return err
		}
		all = append(all, page...)

		if limit > 0 && len(all) >= limit {
			all = all[:limit]
			// Check if there would have been more results
			cursor := extractAfterCursor(body)
			if cursor != "" || len(page) > 0 {
				truncated = true
			}
			break
		}

		cursor := extractAfterCursor(body)
		if cursor == "" {
			break
		}
		after = &cursor
	}

	data, err := json.Marshal(all)
	if err != nil {
		return err
	}

	if err := printOutput(cmd, format, jqExpr, fields, data); err != nil {
		return err
	}

	// Show truncation notice in TTY mode when auto-limit kicked in
	if truncated && autoLimited {
		fmt.Fprintf(os.Stderr, "Showing %d results. Use --limit 0 for all.\n", len(all))
	}

	return nil
}

// extractAfterCursor reads pagination_meta.after from raw JSON,
// avoiding typed struct variance across different API resources.
func extractAfterCursor(body []byte) string {
	var meta struct {
		PaginationMeta *struct {
			After *string `json:"after"`
		} `json:"pagination_meta"`
	}
	if json.Unmarshal(body, &meta) != nil || meta.PaginationMeta == nil || meta.PaginationMeta.After == nil {
		return ""
	}
	return *meta.PaginationMeta.After
}
