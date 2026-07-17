package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// CommandSchema is the JSON schema for a single command.
type CommandSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Usage       string          `json:"usage"`
	Example     string          `json:"example,omitempty"`
	Args        string          `json:"args,omitempty"`
	Flags       []FlagSchema    `json:"flags,omitempty"`
	GlobalFlags []FlagSchema    `json:"global_flags,omitempty"`
	Subcommands []CommandSchema `json:"subcommands,omitempty"`
}

// FlagSchema is the JSON schema for a single flag.
type FlagSchema struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand,omitempty"`
	Type        string `json:"type"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description"`
	Required    bool   `json:"required,omitempty"`
}

var describeCmd = &cobra.Command{
	Use:   "describe [command-path]",
	Short: "Output a JSON schema of all commands and flags",
	Long: `Machine-readable description of the CLI for LLM agent discovery.
Outputs a JSON schema of every command, subcommand, flag, and argument.

Use a dot-separated path to describe a specific command:
  inc describe incidents.list
  inc describe catalog.entries.create`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDescribe,
}

func init() {
	rootCmd.AddCommand(describeCmd)
}

func runDescribe(cmd *cobra.Command, args []string) error {
	target := rootCmd

	if len(args) == 1 {
		parts := strings.Split(args[0], ".")
		for _, part := range parts {
			found := false
			for _, sub := range target.Commands() {
				if sub.Name() == part {
					target = sub
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("unknown command path: %s", args[0])
			}
		}
	}

	schema := buildSchema(target)

	// Add global flags at the root level only
	if target == rootCmd {
		schema.GlobalFlags = buildPersistentFlags(rootCmd)
	}

	out, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, string(out))
	return err
}

func buildSchema(cmd *cobra.Command) CommandSchema {
	schema := CommandSchema{
		Name:        cmd.Name(),
		Description: cmd.Short,
		Usage:       cmd.UseLine(),
		Example:     cmd.Example,
	}

	if cmd.Args != nil {
		schema.Args = cmd.Use
	}

	// Local flags (not inherited)
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		schema.Flags = append(schema.Flags, buildFlagSchema(cmd, f))
	})

	// Subcommands
	for _, sub := range cmd.Commands() {
		if sub.Hidden || sub.Name() == "help" || sub.Name() == "completion" {
			continue
		}
		schema.Subcommands = append(schema.Subcommands, buildSchema(sub))
	}

	return schema
}

func buildFlagSchema(cmd *cobra.Command, f *pflag.Flag) FlagSchema {
	fs := FlagSchema{
		Name:        f.Name,
		Shorthand:   f.Shorthand,
		Type:        f.Value.Type(),
		Default:     f.DefValue,
		Description: f.Usage,
	}

	// Check if cobra marked this flag as required
	if ann := f.Annotations; ann != nil {
		if _, ok := ann[cobra.BashCompOneRequiredFlag]; ok {
			fs.Required = true
		}
	}

	return fs
}

func buildPersistentFlags(cmd *cobra.Command) []FlagSchema {
	var flags []FlagSchema
	cmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		flags = append(flags, buildFlagSchema(cmd, f))
	})
	return flags
}
