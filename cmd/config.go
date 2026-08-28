package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/incident-io/inc/internal/config"
	"github.com/incident-io/inc/internal/output"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CLI configuration",
}

var configGetCmd = &cobra.Command{
	Use:     "get <key>",
	Short:   "Get a config value",
	Example: `  inc config get api_url`,
	Args:    cobra.ExactArgs(1),
	RunE:    runConfigGet,
}

var configSetCmd = &cobra.Command{
	Use:     "set <key> <value>",
	Short:   "Set a config value",
	Example: `  inc config set default_output json`,
	Args:    cobra.ExactArgs(2),
	RunE:    runConfigSet,
}

var configUnsetCmd = &cobra.Command{
	Use:     "unset <key>",
	Short:   "Unset a config value, reverting to the default",
	Example: `  inc config unset api_url`,
	Args:    cobra.ExactArgs(1),
	RunE:    runConfigUnset,
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all config values",
	RunE:  runConfigList,
}

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configUnsetCmd)
	configCmd.AddCommand(configListCmd)
	rootCmd.AddCommand(configCmd)
}

var validKeys = map[string]bool{
	"api_key":        true,
	"api_url":        true,
	"app_url":        true,
	"default_output": true,
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]
	if !validKeys[key] {
		return fmt.Errorf("unknown config key %q. Valid keys: %s", key, strings.Join(validKeysList(), ", "))
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	val := getConfigValue(cfg, key)

	// Mask API key by default
	if key == "api_key" && len(val) > 8 {
		val = val[:4] + "..." + val[len(val)-4:]
	}

	_, err = fmt.Fprintln(os.Stdout, val)
	return err
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	if !validKeys[key] {
		return fmt.Errorf("unknown config key %q. Valid keys: %s", key, strings.Join(validKeysList(), ", "))
	}
	if key == "default_output" && !output.ValidFormat(value) {
		return fmt.Errorf("invalid value %q for default_output. Valid values: %s", value, strings.Join(output.Formats, ", "))
	}

	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}

	setConfigValue(cfg, key, value)

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Set %s in %s\n", key, config.ConfigFilePath())
	return nil
}

func runConfigUnset(cmd *cobra.Command, args []string) error {
	key := args[0]
	if !validKeys[key] {
		return fmt.Errorf("unknown config key %q. Valid keys: %s", key, strings.Join(validKeysList(), ", "))
	}

	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}

	setConfigValue(cfg, key, "")

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Unset %s in %s\n", key, config.ConfigFilePath())
	return nil
}

func runConfigList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	for _, key := range validKeysList() {
		val := getConfigValue(cfg, key)
		if key == "api_key" && len(val) > 8 {
			val = val[:4] + "..." + val[len(val)-4:]
		}
		if _, err := fmt.Fprintf(os.Stdout, "%s=%s\n", key, val); err != nil {
			return err
		}
	}
	return nil
}

func getConfigValue(cfg *config.Config, key string) string {
	switch key {
	case "api_key":
		return cfg.APIKey
	case "api_url":
		return cfg.APIURL
	case "app_url":
		return cfg.AppURL
	case "default_output":
		return cfg.Output
	}
	return ""
}

func setConfigValue(cfg *config.Config, key, value string) {
	switch key {
	case "api_key":
		cfg.APIKey = value
	case "api_url":
		cfg.APIURL = value
	case "app_url":
		cfg.AppURL = value
	case "default_output":
		cfg.Output = value
	}
}

func validKeysList() []string {
	return []string{"api_key", "api_url", "app_url", "default_output"}
}
