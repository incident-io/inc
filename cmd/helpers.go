package cmd

import (
	"encoding/json"
	"net/http"
	"os"

	incident "github.com/incident-io/sdk-go"
	"github.com/spf13/cobra"

	"github.com/incident-io/inc/internal/api"
	"github.com/incident-io/inc/internal/auth"
	"github.com/incident-io/inc/internal/config"
	"github.com/incident-io/inc/internal/output"
)

// newClient creates a typed sdk-go client from the command's flags.
// If --dry-run is set, the client uses a transport that prints requests instead of sending them.
func newClient(cmd *cobra.Command) (*incident.ClientWithResponses, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	flagKey, _ := cmd.Flags().GetString("api-key")
	flagURL, _ := cmd.Flags().GetString("api-url")
	apiKey, apiURL, err := auth.Resolve(cfg, flagKey, flagURL)
	if err != nil {
		return nil, err
	}

	var doer incident.HttpRequestDoer = api.NewRetryDoer()
	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		// No retries under dry-run: the transport prints the request once and
		// short-circuits with ErrDryRun.
		doer = &http.Client{Transport: &api.DryRunTransport{}}
	}

	return incident.New(apiKey,
		incident.WithBaseURL(apiURL),
		incident.WithUserAgent(api.UserAgent(version)),
		incident.WithHTTPClient(doer),
	)
}

// newGenericClient creates a raw HTTP API client from the command's flags.
func newGenericClient(cmd *cobra.Command) (*api.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	flagKey, _ := cmd.Flags().GetString("api-key")
	flagURL, _ := cmd.Flags().GetString("api-url")
	apiKey, apiURL, err := auth.Resolve(cfg, flagKey, flagURL)
	if err != nil {
		return nil, err
	}
	c := api.NewClient(apiURL, apiKey, version)
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	c.DryRun = dryRun
	return c, nil
}

// getOutputFlags reads the shared output flags from a command.
// isQuiet returns true if the --quiet flag is set.
func isQuiet(cmd *cobra.Command) bool {
	q, _ := cmd.Flags().GetBool("quiet")
	return q
}

func getOutputFlags(cmd *cobra.Command) (format, jqExpr, fields string) {
	format, _ = cmd.Flags().GetString("output")
	jqExpr, _ = cmd.Flags().GetString("jq")
	fields, _ = cmd.Flags().GetString("fields")

	configDefault := ""
	if cfg, err := config.Load(); err == nil && output.ValidFormat(cfg.Output) {
		configDefault = cfg.Output
	}
	format = resolveFormat(format, cmd.Flags().Changed("output"), output.IsTTY(), configDefault)

	// A jq filter only makes sense on JSON: rather than silently dropping it
	// on a TTY, let --jq imply JSON unless --output was set explicitly.
	if jqExpr != "" && !cmd.Flags().Changed("output") {
		format = "json"
	}

	return
}

// resolveFormat picks the output format. An explicit --output flag always
// wins; piped output defaults to JSON for scripted usage; otherwise the
// default_output config value applies. An empty configDefault means the
// config was unreadable or held an invalid format, so the flag default wins.
func resolveFormat(flagValue string, explicit, isTTY bool, configDefault string) string {
	if explicit {
		return flagValue
	}
	if !isTTY {
		return "json"
	}
	if configDefault != "" {
		return configDefault
	}
	return flagValue
}

// printOutput wraps output.Print, respecting the --quiet and --limit flags.
// --limit truncates array data so its semantics hold even for endpoints that
// aren't paginated server-side (paginated commands have already truncated by
// the time they get here, which is a no-op).
func printOutput(cmd *cobra.Command, format, jqExpr, fields string, data json.RawMessage) error {
	if isQuiet(cmd) {
		return nil
	}
	return output.Print(os.Stdout, format, jqExpr, fields, applyLimit(cmd, data))
}

// applyLimit truncates a JSON array to --limit items. Non-arrays and
// non-positive limits pass through unchanged.
func applyLimit(cmd *cobra.Command, data json.RawMessage) json.RawMessage {
	limit, _ := cmd.Flags().GetInt("limit")
	if limit <= 0 {
		return data
	}
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil || len(items) <= limit {
		return data
	}
	truncated, err := json.Marshal(items[:limit])
	if err != nil {
		return data
	}
	return truncated
}

// handleAPIResponse checks an oapi-codegen response status code and returns
// a structured APIError on failure. Use this instead of inline fmt.Errorf.
func handleAPIResponse(statusCode int, body []byte) error {
	if statusCode < 400 {
		return nil
	}
	return api.NewAPIErrorFromResponse(statusCode, body)
}
