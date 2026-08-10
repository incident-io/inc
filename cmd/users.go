package cmd

import (
	"github.com/spf13/cobra"

	incident "github.com/incident-io/sdk-go"

	"github.com/incident-io/inc/internal/output"
)

var usersCmd = &cobra.Command{
	Use:   "users",
	Short: "Manage users",
}

var usersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List users",
	RunE:  runUsersList,
}

var usersShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Get a single user",
	Args:  cobra.ExactArgs(1),
	RunE:  runUsersShow,
}

func init() {
	usersListCmd.Flags().String("email", "", "Filter by email address")

	usersCmd.AddCommand(usersListCmd)
	usersCmd.AddCommand(usersShowCmd)
	rootCmd.AddCommand(usersCmd)
}

func runUsersList(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	pageSize, _ := cmd.Flags().GetInt("page-size")
	email, _ := cmd.Flags().GetString("email")

	params := &incident.UsersV2ListParams{}
	ps := int64(pageSize)
	params.PageSize = &ps
	if email != "" {
		params.Email = &email
	}

	return paginate(cmd, "users", func(after *string) ([]byte, int, error) {
		params.After = after
		resp, err := c.UsersV2ListWithResponse(ctx, params)
		if err != nil {
			return nil, 0, err
		}
		return resp.Body, resp.StatusCode(), nil
	}, PaginateOpts{DefaultFields: "id,name,email,role"})
}

func runUsersShow(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	resp, err := c.UsersV2ShowWithResponse(cmd.Context(), id)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	data, err := output.UnwrapEnvelope(resp.Body, "user")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}
