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
	followUpsListCmd.Flags().String("incident-id", "", "Filter by incident ID (ULID, not an INC- reference)")

	followUpsCmd.AddCommand(followUpsListCmd)
	followUpsCmd.AddCommand(followUpsShowCmd)
	rootCmd.AddCommand(followUpsCmd)
}

func runFollowUpsList(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	pageSize, _ := cmd.Flags().GetInt("page-size")
	incidentID, _ := cmd.Flags().GetString("incident-id")

	params := &incident.FollowUpsV3ListParams{}
	ps := int64(pageSize)
	params.PageSize = &ps

	if incidentID != "" {
		params.IncidentId = &incidentID
	}

	return paginate(cmd, "follow_ups", func(after *string) ([]byte, int, error) {
		params.After = after
		resp, err := c.FollowUpsV3ListWithResponse(ctx, params)
		if err != nil {
			return nil, 0, err
		}
		return resp.Body, resp.StatusCode(), nil
	}, PaginateOpts{DefaultFields: "id,title,status,assignee,priority"})
}

func runFollowUpsShow(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	resp, err := c.FollowUpsV3ShowWithResponse(cmd.Context(), id)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	fields = withDefaultFields(format, fields, followUpRecordFields)
	data, err := output.UnwrapEnvelope(resp.Body, "follow_up")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}
