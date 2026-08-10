package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	incident "github.com/incident-io/sdk-go"

	"github.com/incident-io/inc/internal/output"
)

var postmortemsCmd = &cobra.Command{
	Use:   "post-mortems",
	Short: "Manage post-mortem documents",
}

var postmortemsListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List post-mortem documents",
	Example: `  inc post-mortems list --incident-id 01HXYZ`,
	RunE:    runPostmortemsList,
}

var postmortemsShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Get a single post-mortem document",
	Args:  cobra.ExactArgs(1),
	RunE:  runPostmortemsShow,
}

var postmortemsContentCmd = &cobra.Command{
	Use:     "content <id>",
	Short:   "Get the full content of a post-mortem as markdown",
	Example: `  inc post-mortems content 01HXYZ`,
	Args:    cobra.ExactArgs(1),
	RunE:    runPostmortemsContent,
}

func init() {
	postmortemsListCmd.Flags().String("incident-id", "", "Filter by incident ID")

	postmortemsCmd.AddCommand(postmortemsListCmd)
	postmortemsCmd.AddCommand(postmortemsShowCmd)
	postmortemsCmd.AddCommand(postmortemsContentCmd)
	rootCmd.AddCommand(postmortemsCmd)
}

func runPostmortemsList(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	pageSize, _ := cmd.Flags().GetInt("page-size")
	incidentID, _ := cmd.Flags().GetString("incident-id")

	params := &incident.PostmortemDocumentsV1ListParams{}
	ps := int64(pageSize)
	params.PageSize = &ps
	if incidentID != "" {
		params.IncidentId = &incidentID
	}

	return paginate(cmd, "postmortem_documents", func(after *string) ([]byte, int, error) {
		params.After = after
		resp, err := c.PostmortemDocumentsV1ListWithResponse(ctx, params)
		if err != nil {
			return nil, 0, err
		}
		return resp.Body, resp.StatusCode(), nil
	}, PaginateOpts{DefaultFields: "id,title,status,incident_id,created_at"})
}

func runPostmortemsShow(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	resp, err := c.PostmortemDocumentsV1ShowWithResponse(cmd.Context(), id)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	data, err := output.UnwrapEnvelope(resp.Body, "postmortem_document")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}

func runPostmortemsContent(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	resp, err := c.PostmortemDocumentsV1ShowContentWithResponse(cmd.Context(), id)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	if isQuiet(cmd) {
		return nil
	}

	format, _, _ := getOutputFlags(cmd)
	if format == "json" {
		return output.Print(os.Stdout, "json", "", "", resp.Body)
	}

	// For table/default output, just print the markdown directly
	if resp.JSON200 != nil {
		if _, err := fmt.Fprintln(os.Stdout, resp.JSON200.Markdown); err != nil {
			return err
		}
	}
	return nil
}
