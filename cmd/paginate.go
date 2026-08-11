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

// ttyDefaultLimit caps an unqualified list on a terminal, so reading one doesn't
// page through an org's entire history to fill a screen.
//
// It matches the default --page-size deliberately: a cap above it costs a second
// round-trip to fetch a handful of rows that are then thrown away.
const ttyDefaultLimit = 25

// resolveLimit decides how many results to fetch, and reports whether the cap was ours
// rather than the caller's. It keys off whether --limit was passed rather than its value,
// because --limit 0 documents "no limit" (see the flag help in root.go) and Go's zero
// value makes it indistinguishable from an absent flag otherwise.
func resolveLimit(limit int, explicit, isTTY bool) (int, bool) {
	if !explicit && isTTY {
		return ttyDefaultLimit, true
	}
	return limit, false
}

// moreResultsFollow reports whether the API had results beyond the ones we kept, so the
// truncation notice only claims there's more when there is. Both checks are load-bearing:
// a page that fits the limit exactly drops nothing, and only the cursor can say whether it
// was the last page.
func moreResultsFollow(collected, limit int, body []byte) bool {
	return collected > limit || extractAfterCursor(body) != ""
}

// paginate fetches all pages, unwraps each envelope, collects items, and prints.
func paginate(cmd *cobra.Command, envelopeKey string, fetch PageFetcher, opts PaginateOpts) error {
	format, jqExpr, fields := getOutputFlags(cmd)
	limit, _ := cmd.Flags().GetInt("limit")

	fields = withDefaultFields(format, fields, opts.DefaultFields)

	limit, autoLimited := resolveLimit(limit, cmd.Flags().Changed("limit"), output.IsTTY())

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
			truncated = moreResultsFollow(len(all), limit, body)
			all = all[:limit]
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
