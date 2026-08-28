package cmd

import (
	"github.com/spf13/cobra"

	"github.com/incident-io/inc/internal/output"
)

var rolesCmd = &cobra.Command{
	Use:   "roles",
	Short: "Manage incident roles",
}

var rolesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List incident roles",
	RunE:  runRolesList,
}

var rolesShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Get a single incident role",
	Args:  cobra.ExactArgs(1),
	RunE:  runRolesShow,
}

func init() {
	rolesCmd.AddCommand(rolesListCmd)
	rolesCmd.AddCommand(rolesShowCmd)
	rootCmd.AddCommand(rolesCmd)
}

func runRolesList(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	resp, err := c.IncidentRolesV2ListWithResponse(cmd.Context())
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	fields = withDefaultFields(format, fields, "id,name,shortform,role_type")
	data, err := output.UnwrapEnvelope(resp.Body, "incident_roles")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}

func runRolesShow(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	resp, err := c.IncidentRolesV2ShowWithResponse(cmd.Context(), id)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	fields = withDefaultFields(format, fields, incidentRoleRecordFields)
	data, err := output.UnwrapEnvelope(resp.Body, "incident_role")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}
