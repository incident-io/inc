package cmd

import (
	"github.com/spf13/cobra"

	incident "github.com/incident-io/sdk-go"
)

var incidentUpdatesCmd = &cobra.Command{
	Use:   "incident-updates",
	Short: "Manage incident status updates",
}

var incidentUpdatesListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List incident updates",
	Example: `  inc incident-updates list --incident-id 01HXYZ --output json`,
	RunE:    runIncidentUpdatesList,
}

func init() {
	incidentUpdatesListCmd.Flags().String("incident-id", "", "Filter by incident ID")

	incidentUpdatesCmd.AddCommand(incidentUpdatesListCmd)
	rootCmd.AddCommand(incidentUpdatesCmd)
}

func runIncidentUpdatesList(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	pageSize, _ := cmd.Flags().GetInt("page-size")
	incidentID, _ := cmd.Flags().GetString("incident-id")

	params := &incident.IncidentUpdatesV2ListParams{}
	ps := int64(pageSize)
	params.PageSize = &ps
	if incidentID != "" {
		params.IncidentId = &incidentID
	}

	return paginate(cmd, "incident_updates", func(after *string) ([]byte, int, error) {
		params.After = after
		resp, err := c.IncidentUpdatesV2ListWithResponse(ctx, params)
		if err != nil {
			return nil, 0, err
		}
		return resp.Body, resp.StatusCode(), nil
	}, PaginateOpts{DefaultFields: "id,incident_id,new_incident_status,new_severity,created_at"})
}
