package cmd

import (
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/spf13/cobra"

	"github.com/incident-io/inc/internal/api"
	"github.com/incident-io/inc/internal/output"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:           "inc",
	Short:         "CLI for the incident.io API",
	Long:          "A command-line interface for managing incidents, alerts, catalog entries, escalations, schedules, and more via the incident.io API.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	// Output control
	rootCmd.PersistentFlags().StringP("output", "o", "table", "Output format: table or json")
	rootCmd.PersistentFlags().String("jq", "", "JQ filter expression (requires --output json)")
	rootCmd.PersistentFlags().String("fields", "", "Comma-separated list of fields to include in output")

	// API configuration
	rootCmd.PersistentFlags().String("api-key", "", "API key (overrides INCIDENT_API_KEY env var and config file)")
	rootCmd.PersistentFlags().String("api-url", "", "API base URL (overrides INCIDENT_API_URL env var, default: https://api.incident.io)")

	// Pagination
	rootCmd.PersistentFlags().Int("limit", 0, "Maximum number of results to return on list commands (0 = no limit; unset defaults to 25 on a terminal)")
	rootCmd.PersistentFlags().Int("page-size", 25, "Number of results per API request (max 250)")

	// Debug
	rootCmd.PersistentFlags().Bool("dry-run", false, "Print the HTTP request without sending it")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Suppress output on success")

	// Version
	rootCmd.Version = fmt.Sprintf("%s (%s) built %s", version, commit, date)
	rootCmd.SetVersionTemplate("inc version {{.Version}}\n")
}

// Execute runs the root command and returns the appropriate exit code.
func Execute() int {
	cmd, err := rootCmd.ExecuteC()
	if err == nil {
		return 0
	}

	// Dry-run prints the request and returns a sentinel error, not a real
	// failure. This check must come before the url.Error branch: the typed
	// client's dry-run sentinel arrives wrapped in a *url.Error.
	if errors.Is(err, api.ErrDryRun) {
		return 0
	}

	// Errors follow the same format resolution as data output, so piped
	// callers and default_output users get structured JSON errors.
	format, _, _ := getOutputFlags(cmd)

	// Check if the error carries a structured API error with an exit code.
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		output.PrintError(os.Stderr, format, output.ErrorPayload{
			Error:      apiErr.Err,
			Message:    apiErr.Message,
			Suggestion: apiErr.Suggestion,
			RequestID:  apiErr.RequestID,
			Retryable:  apiErr.Retryable,
			APIError:   output.RawBody(apiErr.Body),
			Debug:      apiErr.Debug,
		})
		return apiErr.ExitCode
	}

	// Transport failures from the typed SDK client arrive as *url.Error.
	// Op "parse" means a malformed URL, a user error rather than a network one.
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Op != "parse" {
		output.PrintError(os.Stderr, format, output.ErrorPayload{
			Error:     "network_error",
			Message:   err.Error(),
			Retryable: true,
		})
		return api.ExitCodeNetworkError
	}

	output.PrintError(os.Stderr, format, output.ErrorPayload{
		Error:   "error",
		Message: err.Error(),
	})
	return 1
}
