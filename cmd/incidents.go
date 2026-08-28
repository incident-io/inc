package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	incident "github.com/incident-io/sdk-go"

	"github.com/incident-io/inc/internal/api"
	"github.com/incident-io/inc/internal/output"
)

var incidentsCmd = &cobra.Command{
	Use:   "incidents",
	Short: "Manage incidents",
	Long:  "Manage incidents. Commands taking <id> also accept a reference, like INC-123.",
}

var incidentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List incidents",
	Example: `  inc incidents list --status-category live
  inc incidents list --output json --jq '.[] | {id, name}'
  inc incidents list --severity-id 01HXYZ --sort-by created_at_oldest_first`,
	RunE: runIncidentsList,
}

var incidentsShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Get a single incident, by ID or reference",
	Example: `  inc incidents show INC-123
  inc incidents show 01HXYZ --output json --jq '{name, status: .incident_status.name}'`,
	Args: cobra.ExactArgs(1),
	RunE: runIncidentsShow,
}

var incidentsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an incident",
	Example: `  inc incidents create --name "Database outage" --visibility public
  inc incidents create --name "Staging issue" --severity-id 01HXYZ --mode test`,
	RunE: runIncidentsCreate,
}

var incidentsUpdateCmd = &cobra.Command{
	Use:     "update <id>",
	Short:   "Update an incident, by ID or reference",
	Example: `  inc incidents update INC-123 --name "New name" --notify=false`,
	Args:    cobra.ExactArgs(1),
	RunE:    runIncidentsUpdate,
}

var incidentsCloseCmd = &cobra.Command{
	Use:     "close <id>",
	Short:   "Close an incident, by ID or reference",
	Long:    "Close an incident by setting its status to the first 'closed' category status. Use --status-id to specify an exact status.",
	Example: `  inc incidents close INC-123`,
	Args:    cobra.ExactArgs(1),
	RunE:    runIncidentsClose,
}

func init() {
	incidentsListCmd.Flags().StringSlice("status-category", nil, "Filter by status category (e.g., live, learning, closed)")
	incidentsListCmd.Flags().StringSlice("severity-id", nil, "Filter by severity ID")
	incidentsListCmd.Flags().String("sort-by", "", "Sort order: created_at_newest_first or created_at_oldest_first")

	incidentsCreateCmd.Flags().String("name", "", "Incident name")
	incidentsCreateCmd.Flags().String("visibility", "public", "Visibility: public or private")
	incidentsCreateCmd.Flags().String("severity-id", "", "Severity ID")
	incidentsCreateCmd.Flags().String("incident-type-id", "", "Incident type ID")
	incidentsCreateCmd.Flags().String("mode", "", "Mode: standard, retrospective, test, tutorial")
	incidentsCreateCmd.Flags().String("summary", "", "Incident summary")

	incidentsUpdateCmd.Flags().String("name", "", "New incident name")
	incidentsUpdateCmd.Flags().String("summary", "", "New incident summary")
	incidentsUpdateCmd.Flags().String("severity-id", "", "New severity ID")
	incidentsUpdateCmd.Flags().String("incident-status-id", "", "New incident status ID")
	incidentsUpdateCmd.Flags().Bool("notify", true, "Notify the incident Slack channel")

	incidentsCloseCmd.Flags().String("status-id", "", "Specific status ID to set (default: auto-detect closed status)")
	incidentsCloseCmd.Flags().Bool("notify", true, "Notify the incident Slack channel")

	incidentsCmd.AddCommand(incidentsListCmd)
	incidentsCmd.AddCommand(incidentsShowCmd)
	incidentsCmd.AddCommand(incidentsCreateCmd)
	incidentsCmd.AddCommand(incidentsUpdateCmd)
	incidentsCmd.AddCommand(incidentsCloseCmd)
	rootCmd.AddCommand(incidentsCmd)
}

// normalizeIncidentID turns a reference the user pasted ("INC-84", "#inc-84") into the
// bare number that an /v2/incidents/{id} path segment accepts alongside a ULID. The
// reference is what we print in every table and the only identifier a user has to hand,
// so `inc incidents show INC-84` has to work.
//
// This holds for the {id} path segment only. An incident_id query filter, as used by
// `follow-ups list --incident-id`, requires the ULID and returns an empty list for
// anything else, so don't reach for this there.
//
// Anything that isn't a plain reference passes through untouched, ULIDs included: the
// API is the authority on what's a valid ID.
func normalizeIncidentID(id string) string {
	ref := strings.TrimPrefix(strings.TrimSpace(id), "#")
	if rest, found := strings.CutPrefix(strings.ToLower(ref), "inc-"); found {
		ref = rest
	}
	if _, err := strconv.ParseUint(ref, 10, 64); err != nil {
		return id
	}
	return ref
}

func runIncidentsList(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	pageSize, _ := cmd.Flags().GetInt("page-size")
	statusCategoryFilter, _ := cmd.Flags().GetStringSlice("status-category")
	severityFilter, _ := cmd.Flags().GetStringSlice("severity-id")
	sortBy, _ := cmd.Flags().GetString("sort-by")

	params := &incident.IncidentsV2ListParams{}
	ps := int64(pageSize)
	params.PageSize = &ps

	if sortBy != "" {
		sb := incident.IncidentsV2ListParamsSortBy(sortBy)
		params.SortBy = &sb
	}

	addFilters := func(ctx context.Context, req *http.Request) error {
		q := req.URL.Query()
		for _, v := range statusCategoryFilter {
			q.Add("status_category[one_of]", v)
		}
		for _, v := range severityFilter {
			q.Add("severity[one_of]", v)
		}
		if len(statusCategoryFilter) > 0 || len(severityFilter) > 0 {
			req.URL.RawQuery = q.Encode()
		}
		return nil
	}

	return paginate(cmd, "incidents", func(after *string) ([]byte, int, error) {
		params.After = after
		resp, err := c.IncidentsV2ListWithResponse(ctx, params, addFilters)
		if err != nil {
			return nil, 0, err
		}
		return resp.Body, resp.StatusCode(), nil
	}, PaginateOpts{DefaultFields: "reference,name,incident_status,severity,created_at"})
}

func runIncidentsShow(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	id := normalizeIncidentID(args[0])
	resp, err := c.IncidentsV2ShowWithResponse(cmd.Context(), id)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	fields = withDefaultFields(format, fields, incidentRecordFields)
	data, err := output.UnwrapEnvelope(resp.Body, "incident")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}

func runIncidentsCreate(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	name, _ := cmd.Flags().GetString("name")
	visibility, _ := cmd.Flags().GetString("visibility")
	severityID, _ := cmd.Flags().GetString("severity-id")
	incidentTypeID, _ := cmd.Flags().GetString("incident-type-id")
	mode, _ := cmd.Flags().GetString("mode")
	summary, _ := cmd.Flags().GetString("summary")

	body := incident.IncidentsV2CreateJSONRequestBody{
		IdempotencyKey: uuid.New().String(),
		Visibility:     incident.IncidentsCreatePayloadV2Visibility(visibility),
	}
	if name != "" {
		body.Name = &name
	}
	if severityID != "" {
		body.SeverityId = &severityID
	}
	if incidentTypeID != "" {
		body.IncidentTypeId = &incidentTypeID
	}
	if mode != "" {
		m := incident.IncidentsCreatePayloadV2Mode(mode)
		body.Mode = &m
	}
	if summary != "" {
		body.Summary = &summary
	}

	resp, err := c.IncidentsV2CreateWithResponse(cmd.Context(), body)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	fields = withDefaultFields(format, fields, incidentRecordFields)
	data, err := output.UnwrapEnvelope(resp.Body, "incident")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}

func runIncidentsUpdate(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	id := normalizeIncidentID(args[0])
	name, _ := cmd.Flags().GetString("name")
	summary, _ := cmd.Flags().GetString("summary")
	severityID, _ := cmd.Flags().GetString("severity-id")
	incidentStatusID, _ := cmd.Flags().GetString("incident-status-id")

	edit := incident.IncidentEditPayloadV2{}
	if name != "" {
		edit.Name = &name
	}
	if summary != "" {
		edit.Summary = &summary
	}
	if severityID != "" {
		edit.SeverityId = &severityID
	}
	if incidentStatusID != "" {
		edit.IncidentStatusId = &incidentStatusID
	}

	notify, _ := cmd.Flags().GetBool("notify")
	body := incident.IncidentsV2EditJSONRequestBody{
		Incident:              edit,
		NotifyIncidentChannel: notify,
	}

	resp, err := c.IncidentsV2EditWithResponse(cmd.Context(), id, body)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	fields = withDefaultFields(format, fields, incidentRecordFields)
	data, err := output.UnwrapEnvelope(resp.Body, "incident")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}

func runIncidentsClose(cmd *cobra.Command, args []string) error {
	id := normalizeIncidentID(args[0])
	statusID, _ := cmd.Flags().GetString("status-id")

	// If no explicit status-id, look up the first "closed" status
	if statusID == "" {
		gc, err := newGenericClient(cmd)
		if err != nil {
			return err
		}
		resp, err := gc.Do("GET", "/v1/incident_statuses", nil, nil)
		switch {
		case errors.Is(err, api.ErrDryRun):
			// The lookup preview was printed. Continue with a placeholder so
			// the edit request gets previewed too; the real ID is resolved at
			// runtime.
			statusID = "STATUS_ID_RESOLVED_AT_RUNTIME"
		case err != nil:
			return fmt.Errorf("failed to fetch incident statuses: %w", err)
		default:
			var statuses struct {
				IncidentStatuses []struct {
					ID       string `json:"id"`
					Category string `json:"category"`
				} `json:"incident_statuses"`
			}
			if err := json.Unmarshal(resp, &statuses); err != nil {
				return fmt.Errorf("failed to parse incident statuses: %w", err)
			}
			for _, s := range statuses.IncidentStatuses {
				if s.Category == "closed" {
					statusID = s.ID
					break
				}
			}
			if statusID == "" {
				return api.NewUserError("no status with category 'closed' found. Use --status-id to specify one")
			}
		}
	}

	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	notify, _ := cmd.Flags().GetBool("notify")
	body := incident.IncidentsV2EditJSONRequestBody{
		Incident: incident.IncidentEditPayloadV2{
			IncidentStatusId: &statusID,
		},
		NotifyIncidentChannel: notify,
	}

	resp, err := c.IncidentsV2EditWithResponse(cmd.Context(), id, body)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	fields = withDefaultFields(format, fields, incidentRecordFields)
	data, err := output.UnwrapEnvelope(resp.Body, "incident")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}
