package cmd

import (
	"github.com/spf13/cobra"

	incident "github.com/incident-io/sdk-go"

	"github.com/incident-io/inc/internal/output"
)

var followUpsCmd = &cobra.Command{
	Use:   "follow-ups",
	Short: "Manage follow-up actions",
}

var followUpsListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List follow-ups",
	Example: `  inc follow-ups list --incident-id 01HXYZ`,
	RunE:    runFollowUpsList,
}

var followUpsShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Get a single follow-up",
	Args:  cobra.ExactArgs(1),
	RunE:  runFollowUpsShow,
}

func init() {
	followUpsListCmd.Flags().String("incident-id", "", "Filter by incident ID")

	followUpsCmd.AddCommand(followUpsListCmd)
	followUpsCmd.AddCommand(followUpsShowCmd)
	rootCmd.AddCommand(followUpsCmd)
}

func runFollowUpsList(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	incidentID, _ := cmd.Flags().GetString("incident-id")
	params := &incident.FollowUpsV2ListParams{}
	if incidentID != "" {
		params.IncidentId = &incidentID
	}

	resp, err := c.FollowUpsV2ListWithResponse(cmd.Context(), params)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	fields = withDefaultFields(format, fields, "id,title,status,assignee,priority")
	data, err := output.UnwrapEnvelope(resp.Body, "follow_ups")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}

func runFollowUpsShow(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	resp, err := c.FollowUpsV2ShowWithResponse(cmd.Context(), id)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	data, err := output.UnwrapEnvelope(resp.Body, "follow_up")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}
