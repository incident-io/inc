package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	incident "github.com/incident-io/sdk-go"

	"github.com/incident-io/inc/internal/api"
	"github.com/incident-io/inc/internal/output"
)

var catalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Manage catalog types and entries",
}

// Types

var catalogTypesCmd = &cobra.Command{
	Use:   "types",
	Short: "Manage catalog types",
}

var catalogTypesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List catalog types",
	RunE:  runCatalogTypesList,
}

var catalogTypesShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Get a single catalog type",
	Args:  cobra.ExactArgs(1),
	RunE:  runCatalogTypesShow,
}

var catalogTypesCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a catalog type",
	RunE:  runCatalogTypesCreate,
}

var catalogTypesDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a catalog type",
	Args:  cobra.ExactArgs(1),
	RunE:  runCatalogTypesDelete,
}

// Entries

var catalogEntriesCmd = &cobra.Command{
	Use:   "entries",
	Short: "Manage catalog entries",
}

var catalogEntriesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List catalog entries for a type",
	RunE:  runCatalogEntriesList,
}

var catalogEntriesShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Get a single catalog entry",
	Args:  cobra.ExactArgs(1),
	RunE:  runCatalogEntriesShow,
}

var catalogEntriesCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a catalog entry",
	RunE:  runCatalogEntriesCreate,
}

var catalogEntriesUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a catalog entry",
	Args:  cobra.ExactArgs(1),
	RunE:  runCatalogEntriesUpdate,
}

var catalogEntriesDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a catalog entry",
	Args:  cobra.ExactArgs(1),
	RunE:  runCatalogEntriesDelete,
}

func init() {
	// types
	catalogTypesCreateCmd.Flags().String("name", "", "Type name (required)")
	_ = catalogTypesCreateCmd.MarkFlagRequired("name")
	catalogTypesCreateCmd.Flags().String("description", "", "Type description (required)")
	_ = catalogTypesCreateCmd.MarkFlagRequired("description")
	catalogTypesCmd.AddCommand(catalogTypesListCmd)
	catalogTypesCmd.AddCommand(catalogTypesShowCmd)
	catalogTypesCmd.AddCommand(catalogTypesCreateCmd)
	catalogTypesCmd.AddCommand(catalogTypesDeleteCmd)

	// entries
	catalogEntriesListCmd.Flags().String("type-id", "", "Catalog type ID (required)")
	_ = catalogEntriesListCmd.MarkFlagRequired("type-id")
	catalogEntriesCreateCmd.Flags().String("type-id", "", "Catalog type ID (required)")
	_ = catalogEntriesCreateCmd.MarkFlagRequired("type-id")
	catalogEntriesCreateCmd.Flags().String("name", "", "Entry name (required)")
	_ = catalogEntriesCreateCmd.MarkFlagRequired("name")
	catalogEntriesCreateCmd.Flags().String("attribute-values", "{}", "Attribute values as JSON object")
	catalogEntriesUpdateCmd.Flags().String("name", "", "New entry name (required)")
	_ = catalogEntriesUpdateCmd.MarkFlagRequired("name")
	catalogEntriesUpdateCmd.Flags().String("attribute-values", "{}", "Attribute values as JSON object")
	catalogEntriesCmd.AddCommand(catalogEntriesListCmd)
	catalogEntriesCmd.AddCommand(catalogEntriesShowCmd)
	catalogEntriesCmd.AddCommand(catalogEntriesCreateCmd)
	catalogEntriesCmd.AddCommand(catalogEntriesUpdateCmd)
	catalogEntriesCmd.AddCommand(catalogEntriesDeleteCmd)

	catalogCmd.AddCommand(catalogTypesCmd)
	catalogCmd.AddCommand(catalogEntriesCmd)
	rootCmd.AddCommand(catalogCmd)
}

// Types

func runCatalogTypesList(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	resp, err := c.CatalogV3ListTypesWithResponse(cmd.Context())
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	data, err := output.UnwrapEnvelope(resp.Body, "catalog_types")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}

func runCatalogTypesShow(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	resp, err := c.CatalogV3ShowTypeWithResponse(cmd.Context(), id)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	data, err := output.UnwrapEnvelope(resp.Body, "catalog_type")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}

func runCatalogTypesCreate(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	name, _ := cmd.Flags().GetString("name")
	description, _ := cmd.Flags().GetString("description")

	body := incident.CatalogV3CreateTypeJSONRequestBody{
		Name:        name,
		Description: description,
	}

	resp, err := c.CatalogV3CreateTypeWithResponse(cmd.Context(), body)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	data, err := output.UnwrapEnvelope(resp.Body, "catalog_type")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}

func runCatalogTypesDelete(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	resp, err := c.CatalogV3DestroyTypeWithResponse(cmd.Context(), id)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Deleted catalog type %s\n", id)
	return nil
}

// Entries

func runCatalogEntriesList(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	pageSize, _ := cmd.Flags().GetInt("page-size")
	catalogTypeID, _ := cmd.Flags().GetString("type-id")

	params := &incident.CatalogV3ListEntriesParams{
		CatalogTypeId: catalogTypeID,
		PageSize:      int64(pageSize),
	}

	return paginate(cmd, "catalog_entries", func(after *string) ([]byte, int, error) {
		params.After = after
		resp, err := c.CatalogV3ListEntriesWithResponse(ctx, params)
		if err != nil {
			return nil, 0, err
		}
		return resp.Body, resp.StatusCode(), nil
	})
}

func runCatalogEntriesShow(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	resp, err := c.CatalogV3ShowEntryWithResponse(cmd.Context(), id, nil)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	data, err := output.UnwrapEnvelope(resp.Body, "catalog_entry")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}

func runCatalogEntriesCreate(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	typeID, _ := cmd.Flags().GetString("type-id")
	name, _ := cmd.Flags().GetString("name")
	attrJSON, _ := cmd.Flags().GetString("attribute-values")

	var attrs map[string]incident.CatalogEngineParamBindingPayloadV3
	if err := json.Unmarshal([]byte(attrJSON), &attrs); err != nil {
		return api.NewUserError(fmt.Sprintf("invalid --attribute-values JSON: %s", err))
	}

	body := incident.CatalogV3CreateEntryJSONRequestBody{
		CatalogTypeId:   typeID,
		Name:            name,
		AttributeValues: attrs,
	}

	resp, err := c.CatalogV3CreateEntryWithResponse(cmd.Context(), body)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	data, err := output.UnwrapEnvelope(resp.Body, "catalog_entry")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}

func runCatalogEntriesUpdate(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	name, _ := cmd.Flags().GetString("name")
	attrJSON, _ := cmd.Flags().GetString("attribute-values")

	var attrs map[string]incident.CatalogEngineParamBindingPayloadV3
	if err := json.Unmarshal([]byte(attrJSON), &attrs); err != nil {
		return api.NewUserError(fmt.Sprintf("invalid --attribute-values JSON: %s", err))
	}

	body := incident.CatalogV3UpdateEntryJSONRequestBody{
		Name:            name,
		AttributeValues: attrs,
	}

	resp, err := c.CatalogV3UpdateEntryWithResponse(cmd.Context(), id, body)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	data, err := output.UnwrapEnvelope(resp.Body, "catalog_entry")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}

func runCatalogEntriesDelete(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	resp, err := c.CatalogV3DestroyEntryWithResponse(cmd.Context(), id)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Deleted catalog entry %s\n", id)
	return nil
}
