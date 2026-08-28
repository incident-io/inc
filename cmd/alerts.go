package cmd

import (
	"context"
	"net/http"

	"github.com/spf13/cobra"

	incident "github.com/incident-io/sdk-go"

	"github.com/incident-io/inc/internal/output"
)

var alertsCmd = &cobra.Command{
	Use:   "alerts",
	Short: "Manage alerts",
}

var alertsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List alerts",
	RunE:  runAlertsList,
}

var alertsShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Get a single alert",
	Args:  cobra.ExactArgs(1),
	RunE:  runAlertsShow,
}

func init() {
	alertsListCmd.Flags().StringSlice("status", nil, "Filter by status (e.g., firing, resolved)")

	alertsCmd.AddCommand(alertsListCmd)
	alertsCmd.AddCommand(alertsShowCmd)
	rootCmd.AddCommand(alertsCmd)
}

func runAlertsList(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	pageSize, _ := cmd.Flags().GetInt("page-size")
	statusFilter, _ := cmd.Flags().GetStringSlice("status")

	// Alerts max page_size is 50
	if pageSize > 50 {
		pageSize = 50
	}
	params := &incident.AlertsV2ListParams{PageSize: int64(pageSize)}

	addFilters := func(ctx context.Context, req *http.Request) error {
		if len(statusFilter) > 0 {
			q := req.URL.Query()
			for _, v := range statusFilter {
				q.Add("status[one_of]", v)
			}
			req.URL.RawQuery = q.Encode()
		}
		return nil
	}

	return paginate(cmd, "alerts", func(after *string) ([]byte, int, error) {
		params.After = after
		resp, err := c.AlertsV2ListWithResponse(ctx, params, addFilters)
		if err != nil {
			return nil, 0, err
		}
		return resp.Body, resp.StatusCode(), nil
	}, PaginateOpts{DefaultFields: "id,title,status,created_at"})
}

func runAlertsShow(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	resp, err := c.AlertsV2ShowWithResponse(cmd.Context(), id)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	fields = withDefaultFields(format, fields, alertRecordFields)
	data, err := output.UnwrapEnvelope(resp.Body, "alert")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}
