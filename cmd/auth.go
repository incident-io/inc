package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

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
	Short: "Set your API key",
	Long:  "Paste your incident.io API key. In non-TTY mode, reads from stdin.",
	RunE:  runAuthLogin,
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
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authTokenCmd)
	rootCmd.AddCommand(authCmd)
}

func runAuthLogin(cmd *cobra.Command, args []string) error {
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

	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}
	cfg.APIKey = token
	if cfg.APIURL == "" {
		cfg.APIURL = config.DefaultAPIURL
	}
	if cfg.Output == "" {
		cfg.Output = config.DefaultOutput
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Fprintf(os.Stderr, "API key saved to %s\n", config.ConfigFilePath())
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
