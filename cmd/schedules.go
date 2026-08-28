package cmd

import (
	"strings"
	"time"

	"github.com/spf13/cobra"

	incident "github.com/incident-io/sdk-go"

	"github.com/incident-io/inc/internal/output"
)

var schedulesCmd = &cobra.Command{
	Use:   "schedules",
	Short: "Manage on-call schedules",
}

var schedulesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List schedules",
	RunE:  runSchedulesList,
}

var schedulesShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Get a single schedule",
	Args:  cobra.ExactArgs(1),
	RunE:  runSchedulesShow,
}

var schedulesEntriesCmd = &cobra.Command{
	Use:   "entries <schedule-id>",
	Short: "List who is on call for a schedule",
	Args:  cobra.ExactArgs(1),
	RunE:  runSchedulesEntries,
}

var schedulesOverrideCmd = &cobra.Command{
	Use:   "override",
	Short: "Create a schedule override (swap a shift)",
	RunE:  runSchedulesOverride,
}

func init() {
	schedulesEntriesCmd.Flags().String("from", "", "Start of window (ISO 8601, default: now)")
	schedulesEntriesCmd.Flags().String("until", "", "End of window (ISO 8601, default: 24h from now)")

	schedulesOverrideCmd.Flags().String("schedule-id", "", "Schedule ID (required)")
	_ = schedulesOverrideCmd.MarkFlagRequired("schedule-id")
	schedulesOverrideCmd.Flags().String("rotation-id", "", "Rotation ID (required)")
	_ = schedulesOverrideCmd.MarkFlagRequired("rotation-id")
	schedulesOverrideCmd.Flags().String("layer-id", "", "Layer ID (required)")
	_ = schedulesOverrideCmd.MarkFlagRequired("layer-id")
	schedulesOverrideCmd.Flags().String("user", "", "User ID or email (required)")
	_ = schedulesOverrideCmd.MarkFlagRequired("user")
	schedulesOverrideCmd.Flags().String("start", "", "Override start time (ISO 8601, required)")
	_ = schedulesOverrideCmd.MarkFlagRequired("start")
	schedulesOverrideCmd.Flags().String("end", "", "Override end time (ISO 8601, required)")
	_ = schedulesOverrideCmd.MarkFlagRequired("end")

	schedulesCmd.AddCommand(schedulesListCmd)
	schedulesCmd.AddCommand(schedulesShowCmd)
	schedulesCmd.AddCommand(schedulesEntriesCmd)
	schedulesCmd.AddCommand(schedulesOverrideCmd)
	rootCmd.AddCommand(schedulesCmd)
}

func runSchedulesList(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	pageSize, _ := cmd.Flags().GetInt("page-size")
	params := &incident.SchedulesV2ListParams{}
	ps := int64(pageSize)
	params.PageSize = &ps

	return paginate(cmd, "schedules", func(after *string) ([]byte, int, error) {
		params.After = after
		resp, err := c.SchedulesV2ListWithResponse(ctx, params)
		if err != nil {
			return nil, 0, err
		}
		return resp.Body, resp.StatusCode(), nil
	}, PaginateOpts{DefaultFields: "id,name,timezone,created_at"})
}

func runSchedulesShow(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	resp, err := c.SchedulesV2ShowWithResponse(cmd.Context(), id)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	fields = withDefaultFields(format, fields, scheduleRecordFields)
	data, err := output.UnwrapEnvelope(resp.Body, "schedule")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}

func runSchedulesEntries(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	scheduleID := args[0]
	params := &incident.SchedulesV2ListScheduleEntriesParams{
		ScheduleId: scheduleID,
	}

	fromStr, _ := cmd.Flags().GetString("from")
	untilStr, _ := cmd.Flags().GetString("until")

	if fromStr != "" {
		t, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			return err
		}
		// The API accepts either a timestamp or an opaque pagination cursor
		// here, so the SDK types this as a string. RFC3339Nano matches how
		// time.Time query params were serialized before.
		s := t.Format(time.RFC3339Nano)
		params.EntryWindowStart = &s
	}
	if untilStr != "" {
		t, err := time.Parse(time.RFC3339, untilStr)
		if err != nil {
			return err
		}
		params.EntryWindowEnd = &t
	}

	resp, err := c.SchedulesV2ListScheduleEntriesWithResponse(cmd.Context(), params)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	data, err := output.UnwrapEnvelope(resp.Body, "schedule_entries")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}

func runSchedulesOverride(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	scheduleID, _ := cmd.Flags().GetString("schedule-id")
	rotationID, _ := cmd.Flags().GetString("rotation-id")
	layerID, _ := cmd.Flags().GetString("layer-id")
	userRef, _ := cmd.Flags().GetString("user")
	startStr, _ := cmd.Flags().GetString("start")
	endStr, _ := cmd.Flags().GetString("end")

	startAt, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return err
	}
	endAt, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return err
	}

	// Determine if user ref is an email or ID
	user := incident.UserReferencePayloadV2{}
	if strings.Contains(userRef, "@") {
		user.Email = &userRef
	} else {
		user.Id = &userRef
	}

	body := incident.SchedulesV2CreateOverrideJSONRequestBody{
		ScheduleId: scheduleID,
		RotationId: rotationID,
		LayerId:    layerID,
		User:       user,
		StartAt:    startAt,
		EndAt:      endAt,
	}

	resp, err := c.SchedulesV2CreateOverrideWithResponse(cmd.Context(), body)
	if err != nil {
		return err
	}
	if err := handleAPIResponse(resp.StatusCode(), resp.Body); err != nil {
		return err
	}

	format, jqExpr, fields := getOutputFlags(cmd)
	data, err := output.UnwrapEnvelope(resp.Body, "override")
	if err != nil {
		return err
	}
	return printOutput(cmd, format, jqExpr, fields, data)
}
