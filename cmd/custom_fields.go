package cmd

import (
	"github.com/spf13/cobra"

	"github.com/incident-io/inc/internal/output"
)

var customFieldsCmd = &cobra.Command{
	Use:   "custom-fields",
	Short: "Manage custom fields",
}

var customFieldsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List custom fields",
	RunE:  runCustomFieldsList,
}

var customFieldsShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Get a single custom field",
	Args:  cobra.ExactArgs(1),
	RunE:  runCustomFieldsShow,
}

func init() {
	customFieldsCmd.AddCommand(customFieldsListCmd)
	customFieldsCmd.AddCommand(customFieldsShowCmd)
	rootCmd.AddCommand(customFieldsCmd)
}

func runCustomFieldsList(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	resp, err := c.CustomFieldsV2ListWithResponse(cmd.Context())
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	data, err := output.UnwrapEnvelope(resp.Body, "custom_fields")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}

func runCustomFieldsShow(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	resp, err := c.CustomFieldsV2ShowWithResponse(cmd.Context(), id)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	data, err := output.UnwrapEnvelope(resp.Body, "custom_field")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}
