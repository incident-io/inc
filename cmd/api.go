package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/incident-io/inc/internal/api"
	"github.com/incident-io/inc/internal/output"
)

var apiCmd = &cobra.Command{
	Use:   "api <METHOD> <PATH>",
	Short: "Hit any incident.io API endpoint directly",
	Long: `Make an authenticated request to any incident.io API endpoint.

Examples:
  inc api GET /v2/incidents
  inc api GET /v2/incidents --jq '.incidents[] | {id, name}'
  inc api GET /v2/incidents --field status[one_of]=live --field page_size=10
  inc api POST /v2/incidents --field name="Database outage" --field severity_id=01HXYZ
  echo '{"name": "outage"}' | inc api POST /v2/incidents --input -
  inc api GET /v2/incidents --paginate --jq '.incidents[]'`,
	Args: cobra.ExactArgs(2),
	RunE: runAPI,
}

func init() {
	apiCmd.Flags().StringArray("field", nil, "Add a field: key=value (query param for GET, body field for POST/PUT)")
	apiCmd.Flags().String("input", "", "Read body from file (use '-' for stdin)")
	apiCmd.Flags().Bool("paginate", false, "Auto-paginate through all results")
	rootCmd.AddCommand(apiCmd)
}

func runAPI(cmd *cobra.Command, args []string) error {
	method := strings.ToUpper(args[0])
	path := args[1]

	client, err := newGenericClient(cmd)
	if err != nil {
		return err
	}

	// Parse --field flags
	fieldFlags, _ := cmd.Flags().GetStringArray("field")
	query, bodyFields := parseFields(method, fieldFlags)

	// Build request body
	var body io.Reader
	inputFlag, _ := cmd.Flags().GetString("input")
	if inputFlag == "-" {
		body = os.Stdin
	} else if inputFlag != "" {
		f, err := os.Open(inputFlag)
		if err != nil {
			return fmt.Errorf("failed to open input file: %w", err)
		}
		defer func() { _ = f.Close() }()
		body = f
	} else if len(bodyFields) > 0 {
		bodyJSON, err := json.Marshal(bodyFields)
		if err != nil {
			return err
		}
		body = strings.NewReader(string(bodyJSON))
	}

	paginate, _ := cmd.Flags().GetBool("paginate")
	outputFmt, jqExpr, fields := getOutputFlags(cmd)

	if paginate {
		return runPaginated(client, method, path, query, body, outputFmt, jqExpr, fields, isQuiet(cmd))
	}

	resp, err := client.Do(method, path, query, body)
	if err != nil {
		return err
	}

	if isQuiet(cmd) {
		return nil
	}
	return output.Print(os.Stdout, outputFmt, jqExpr, fields, resp)
}

func runPaginated(client *api.Client, method, path string, query map[string][]string, body io.Reader, outputFmt, jqExpr, fields string, quiet bool) error {
	if query == nil {
		query = make(map[string][]string)
	}

	// Collect all pages into a single JSON array for valid output.
	allPages := make([]json.RawMessage, 0)

	for {
		resp, err := client.Do(method, path, query, body)
		if err != nil {
			return err
		}

		allPages = append(allPages, resp)

		// Check for pagination_meta.after
		var meta struct {
			PaginationMeta *struct {
				After string `json:"after"`
			} `json:"pagination_meta"`
		}
		if err := json.Unmarshal(resp, &meta); err != nil {
			break
		}
		if meta.PaginationMeta == nil || meta.PaginationMeta.After == "" {
			break
		}

		query["after"] = []string{meta.PaginationMeta.After}
		body = nil // only send body on first request
	}

	if quiet {
		return nil
	}

	// With a jq filter, apply it per page (like gh api --paginate) so
	// expressions written for the endpoint's envelope work across pages.
	if jqExpr != "" && outputFmt == "json" {
		for _, page := range allPages {
			if err := output.Print(os.Stdout, outputFmt, jqExpr, fields, page); err != nil {
				return err
			}
		}
		return nil
	}

	// Single page: output as-is (no wrapping array)
	if len(allPages) == 1 {
		return output.Print(os.Stdout, outputFmt, jqExpr, fields, allPages[0])
	}

	// Multiple pages: wrap in an array
	data, err := json.Marshal(allPages)
	if err != nil {
		return err
	}
	return output.Print(os.Stdout, outputFmt, jqExpr, fields, data)
}

// parseFields splits --field flags into query params (for GET) or body fields (for POST/PUT/PATCH).
func parseFields(method string, flags []string) (query map[string][]string, bodyFields map[string]any) {
	query = make(map[string][]string)
	bodyFields = make(map[string]any)

	for _, f := range flags {
		key, value, ok := strings.Cut(f, "=")
		if !ok {
			continue
		}
		if method == "GET" || method == "HEAD" {
			query[key] = append(query[key], value)
		} else {
			bodyFields[key] = value
		}
	}

	return query, bodyFields
}
