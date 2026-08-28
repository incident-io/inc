package cmd

import (
	"github.com/spf13/cobra"

	"github.com/incident-io/inc/internal/output"
)

var severitiesCmd = &cobra.Command{
	Use:   "severities",
	Short: "Manage severities",
}

var severitiesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List severities",
	RunE:  runSeveritiesList,
}

var severitiesShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Get a single severity",
	Args:  cobra.ExactArgs(1),
	RunE:  runSeveritiesShow,
}

func init() {
	severitiesCmd.AddCommand(severitiesListCmd)
	severitiesCmd.AddCommand(severitiesShowCmd)
	rootCmd.AddCommand(severitiesCmd)
}

func runSeveritiesList(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	resp, err := c.SeveritiesV1ListWithResponse(cmd.Context())
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	fields = withDefaultFields(format, fields, "id,name,rank")
	data, err := output.UnwrapEnvelope(resp.Body, "severities")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}

func runSeveritiesShow(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	resp, err := c.SeveritiesV1ShowWithResponse(cmd.Context(), id)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	fields = withDefaultFields(format, fields, severityRecordFields)
	data, err := output.UnwrapEnvelope(resp.Body, "severity")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}
