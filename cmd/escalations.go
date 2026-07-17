package cmd

import (
	"github.com/google/uuid"
	"github.com/spf13/cobra"

	incident "github.com/incident-io/sdk-go"

	"github.com/incident-io/inc/internal/output"
)

var escalationsCmd = &cobra.Command{
	Use:   "escalations",
	Short: "Manage escalations and escalation paths",
}

// Live escalations

var escalationsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List live escalations",
	RunE:  runEscalationsList,
}

var escalationsShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Get a single escalation",
	Args:  cobra.ExactArgs(1),
	RunE:  runEscalationsShow,
}

var escalationsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Trigger an escalation",
	RunE:  runEscalationsCreate,
}

// Escalation paths (config)

var escalationPathsCmd = &cobra.Command{
	Use:   "paths",
	Short: "Manage escalation path configurations",
}

var escalationPathsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List escalation paths",
	RunE:  runEscalationPathsList,
}

var escalationPathsShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Get a single escalation path",
	Args:  cobra.ExactArgs(1),
	RunE:  runEscalationPathsShow,
}

func init() {
	escalationsCreateCmd.Flags().String("title", "", "Escalation title (required)")
	_ = escalationsCreateCmd.MarkFlagRequired("title")
	escalationsCreateCmd.Flags().String("escalation-path-id", "", "Escalation path ID to follow")
	escalationsCreateCmd.Flags().String("description", "", "Additional details")

	escalationsCmd.AddCommand(escalationsListCmd)
	escalationsCmd.AddCommand(escalationsShowCmd)
	escalationsCmd.AddCommand(escalationsCreateCmd)

	escalationPathsCmd.AddCommand(escalationPathsListCmd)
	escalationPathsCmd.AddCommand(escalationPathsShowCmd)
	escalationsCmd.AddCommand(escalationPathsCmd)

	rootCmd.AddCommand(escalationsCmd)
}

func runEscalationsList(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	pageSize, _ := cmd.Flags().GetInt("page-size")
	params := &incident.EscalationsV2ListParams{}
	ps := int64(pageSize)
	params.PageSize = &ps

	return paginate(cmd, "escalations", func(after *string) ([]byte, int, error) {
		params.After = after
		resp, err := c.EscalationsV2ListWithResponse(ctx, params)
		if err != nil {
			return nil, 0, err
		}
		return resp.Body, resp.StatusCode(), nil
	})
}

func runEscalationsShow(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	resp, err := c.EscalationsV2ShowWithResponse(cmd.Context(), id)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	data, err := output.UnwrapEnvelope(resp.Body, "escalation")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}

func runEscalationsCreate(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	title, _ := cmd.Flags().GetString("title")
	pathID, _ := cmd.Flags().GetString("escalation-path-id")
	description, _ := cmd.Flags().GetString("description")

	body := incident.EscalationsV2CreateJSONRequestBody{
		IdempotencyKey: uuid.New().String(),
		Title:          title,
	}
	if pathID != "" {
		body.EscalationPathId = &pathID
	}
	if description != "" {
		body.Description = &description
	}

	resp, err := c.EscalationsV2CreateWithResponse(cmd.Context(), body)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	data, err := output.UnwrapEnvelope(resp.Body, "escalation")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}

func runEscalationPathsList(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	pageSize, _ := cmd.Flags().GetInt("page-size")
	params := &incident.EscalationsV2ListPathsParams{}
	ps := int64(pageSize)
	params.PageSize = &ps

	return paginate(cmd, "escalation_paths", func(after *string) ([]byte, int, error) {
		params.After = after
		resp, err := c.EscalationsV2ListPathsWithResponse(ctx, params)
		if err != nil {
			return nil, 0, err
		}
		return resp.Body, resp.StatusCode(), nil
	})
}

func runEscalationPathsShow(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	resp, err := c.EscalationsV2ShowPathWithResponse(cmd.Context(), id)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	data, err := output.UnwrapEnvelope(resp.Body, "escalation_path")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}
