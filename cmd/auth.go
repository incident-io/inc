package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/incident-io/inc/internal/auth"
	"github.com/incident-io/inc/internal/config"
	"github.com/incident-io/inc/internal/output"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to incident.io",
	Long: "Log in via your browser (OAuth). Pass --paste to enter an API key " +
		"instead. In non-TTY mode, reads an API key from stdin.",
	RunE: runAuthLogin,
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if authentication is working",
	RunE:  runAuthStatus,
}

var authTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Print the current API key to stdout",
	RunE:  runAuthToken,
}

func init() {
	authLoginCmd.Flags().Bool("paste", false, "Paste an API key instead of logging in via the browser")
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authTokenCmd)
	rootCmd.AddCommand(authCmd)
}

func runAuthLogin(cmd *cobra.Command, args []string) error {
	// Browser (OAuth) login is the interactive default; --paste and non-TTY
	// mode keep the original API-key path for CI and restricted environments.
	paste, _ := cmd.Flags().GetBool("paste")
	if output.IsTTY() && !paste {
		return runAuthLoginOAuth(cmd)
	}

	var token string

	if output.IsTTY() {
		fmt.Fprint(os.Stderr, "Paste your incident.io API key: ")
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr) // newline after hidden input
		if err != nil {
			return fmt.Errorf("failed to read API key: %w", err)
		}
		token = strings.TrimSpace(string(raw))
	} else {
		// Non-TTY: read from stdin as-is
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read API key from stdin: %w", err)
		}
		token = strings.TrimSpace(string(raw))
	}
	if token == "" {
		return fmt.Errorf("no API key provided")
	}

	cfg := config.LoadOrDefaults()
	// Logging in replaces the stored credential: clear any OAuth token so the
	// pasted key actually takes effect (and vice versa in the OAuth path).
	cfg.APIKey = token
	cfg.OAuthToken = ""
	cfg.OAuthExpiresAt = ""

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Fprintf(os.Stderr, "API key saved to %s\n", config.ConfigFilePath())
	return nil
}

func runAuthLoginOAuth(cmd *cobra.Command) error {
	cfg := config.LoadOrDefaults()

	token, err := auth.OAuthLogin(cmd.Context(), cfg.AppURL, version, os.Stderr)
	if err != nil {
		return err
	}

	// Logging in replaces the stored credential: clear any saved API key so
	// the new token actually takes effect. INCIDENT_API_KEY in the
	// environment still wins at resolution time, so warn if one is set.
	cfg.APIKey = ""
	cfg.OAuthToken = token.AccessToken
	cfg.OAuthExpiresAt = token.ExpiresAt.UTC().Format(time.RFC3339)

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Logged in. Token saved to %s (expires %s).\n",
		config.ConfigFilePath(), token.ExpiresAt.Local().Format("2 Jan 2006"))
	if os.Getenv("INCIDENT_API_KEY") != "" {
		fmt.Fprintln(os.Stderr, "Note: INCIDENT_API_KEY is set in your environment and takes precedence over this login.")
	}
	return nil
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	gc, err := newGenericClient(cmd)
	if err != nil {
		return err
	}

	resp, err := gc.Do("GET", "/v1/identity", nil, nil)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	if isQuiet(cmd) {
		return nil
	}

	format, jqExpr, fields := getOutputFlags(cmd)

	if format == "json" {
		return output.Print(os.Stdout, "json", jqExpr, fields, resp)
	}

	var parsed struct {
		Identity struct {
			Name         string `json:"name"`
			DashboardURL string `json:"dashboard_url"`
		} `json:"identity"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return output.Print(os.Stdout, "json", "", "", resp)
	}

	_, err = fmt.Fprintf(os.Stdout, "Authenticated as %s\n", parsed.Identity.Name)
	return err
}

func runAuthToken(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	flagKey, _ := cmd.Flags().GetString("api-key")
	apiKey, _, err := auth.Resolve(cfg, flagKey, "")
	if err != nil {
		return err
	}

	if isQuiet(cmd) {
		return nil
	}
	_, err = fmt.Fprintln(os.Stdout, apiKey)
	return err
}
