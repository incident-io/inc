package cmd

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestIncidentsList_PaginatesAllPages(t *testing.T) {
	srv := newStubServer(t)
	srv.respond("GET /v2/incidents",
		`{"incidents":[{"id":"01A","name":"one"},{"id":"01B","name":"two"}],"pagination_meta":{"after":"cursor-1","page_size":25}}`,
		`{"incidents":[{"id":"01C","name":"three"}],"pagination_meta":{"page_size":25}}`,
	)

	res := runCommand(t, srv.args("incidents", "list")...)

	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &items); err != nil {
		t.Fatalf("stdout is not a JSON array: %v in %s", err, res.stdout)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 incidents across pages, got %d", len(items))
	}
	if len(srv.requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(srv.requests))
	}
	if !strings.Contains(srv.requests[1].Query, "after=cursor-1") {
		t.Errorf("second request should carry the cursor, got query: %s", srv.requests[1].Query)
	}
	if !strings.Contains(srv.requests[0].Query, "page_size=25") {
		t.Errorf("expected default page_size=25, got query: %s", srv.requests[0].Query)
	}
}

func TestIncidentsList_LimitTruncates(t *testing.T) {
	srv := newStubServer(t)
	srv.respond("GET /v2/incidents",
		`{"incidents":[{"id":"01A"},{"id":"01B"}],"pagination_meta":{"after":"cursor-1"}}`,
	)

	res := runCommand(t, srv.args("incidents", "list", "--limit", "1")...)

	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Errorf("expected --limit 1 to truncate to 1 item, got %d", len(items))
	}
	if len(srv.requests) != 1 {
		t.Errorf("expected limit to stop after 1 request, got %d", len(srv.requests))
	}
	// The truncation notice is TTY-only; piped output must stay clean.
	if res.stderr != "" {
		t.Errorf("expected no stderr when piped, got: %s", res.stderr)
	}
}

func TestIncidentsList_FiltersInQuery(t *testing.T) {
	srv := newStubServer(t)
	srv.respond("GET /v2/incidents", `{"incidents":[]}`)

	res := runCommand(t, srv.args("incidents", "list", "--status-category", "live")...)

	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}
	q := srv.requests[0].Query
	if !strings.Contains(q, "status_category%5Bone_of%5D=live") {
		t.Errorf("expected status_category[one_of]=live in query, got: %s", q)
	}
}

func TestIncidentsCreate_SendsBodyAndUnwrapsEnvelope(t *testing.T) {
	srv := newStubServer(t)
	srv.respond("POST /v2/incidents", `{"incident":{"id":"01NEW","name":"Database outage"}}`)

	res := runCommand(t, srv.args("incidents", "create", "--name", "Database outage", "--visibility", "public")...)

	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(srv.requests[0].Body), &sent); err != nil {
		t.Fatal(err)
	}
	if sent["name"] != "Database outage" || sent["visibility"] != "public" {
		t.Errorf("unexpected request body: %s", srv.requests[0].Body)
	}
	if key, _ := sent["idempotency_key"].(string); key == "" {
		t.Error("expected a generated idempotency_key in the request body")
	}

	// Output is the unwrapped incident, not the envelope.
	var out map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
		t.Fatal(err)
	}
	if out["id"] != "01NEW" {
		t.Errorf("expected unwrapped incident object, got: %s", res.stdout)
	}
}

func TestIncidentsCreate_DryRunPrintsRequestAndSendsNothing(t *testing.T) {
	srv := newStubServer(t)

	res := runCommand(t, srv.args("incidents", "create", "--name", "Test", "--visibility", "public", "--dry-run")...)

	if res.exit != 0 {
		t.Fatalf("dry-run should exit 0, got %d, stderr: %s", res.exit, res.stderr)
	}
	if len(srv.requests) != 0 {
		t.Errorf("dry-run must not send requests, server saw %d", len(srv.requests))
	}
	if !strings.Contains(res.stderr, `"method": "POST"`) || !strings.Contains(res.stderr, "/v2/incidents") {
		t.Errorf("expected POST preview on stderr, got: %s", res.stderr)
	}
	if strings.Contains(res.stderr, "test-key") {
		t.Error("dry-run preview must not leak the API key")
	}
}

func TestIncidentsClose_ResolvesClosedStatusThenEdits(t *testing.T) {
	srv := newStubServer(t)
	srv.respond("GET /v1/incident_statuses",
		`{"incident_statuses":[{"id":"01LIVE","category":"live"},{"id":"01CLOSED","category":"closed"}]}`)
	srv.respond("POST /v2/incidents/01ABC/actions/edit", `{"incident":{"id":"01ABC","name":"done"}}`)

	res := runCommand(t, srv.args("incidents", "close", "01ABC")...)

	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}
	if len(srv.requests) != 2 {
		t.Fatalf("expected preflight + edit, got %d requests", len(srv.requests))
	}
	if !strings.Contains(srv.requests[1].Body, "01CLOSED") {
		t.Errorf("edit should use the resolved closed status, body: %s", srv.requests[1].Body)
	}
}

func TestIncidentsClose_DryRunPreviewsBothRequests(t *testing.T) {
	srv := newStubServer(t)

	res := runCommand(t, srv.args("incidents", "close", "01ABC", "--dry-run")...)

	if res.exit != 0 {
		t.Fatalf("dry-run should exit 0, got %d, stderr: %s", res.exit, res.stderr)
	}
	if len(srv.requests) != 0 {
		t.Errorf("dry-run must not send requests, server saw %d", len(srv.requests))
	}
	if !strings.Contains(res.stderr, "/v1/incident_statuses") {
		t.Errorf("expected the status lookup preview, got: %s", res.stderr)
	}
	if !strings.Contains(res.stderr, "STATUS_ID_RESOLVED_AT_RUNTIME") {
		t.Errorf("expected the edit preview with placeholder status, got: %s", res.stderr)
	}
}

func TestSeveritiesList_UnwrapsEnvelope(t *testing.T) {
	srv := newStubServer(t)
	srv.respond("GET /v1/severities", `{"severities":[{"id":"01SEV","name":"Major"}]}`)

	res := runCommand(t, srv.args("severities", "list")...)

	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0]["name"] != "Major" {
		t.Errorf("expected unwrapped severities array, got: %s", res.stdout)
	}
}

func TestEscalationsList_TableUsesCuratedColumns(t *testing.T) {
	srv := newStubServer(t)
	srv.respond("GET /v2/escalations",
		`{"escalations":[{"id":"01ESC","title":"Queue build-up","status":"resolved",
		  "priority":{"name":"Urgent"},"created_at":"2026-07-31T09:23:53.868Z",
		  "escalation_path_id":"01PATH","creator":{"alert":{"id":"01AL","title":"noisy"}},
		  "events":[{"id":"e1"}],"related_alerts":[],"related_incidents":[]}],
		  "pagination_meta":{}}`)

	res := runCommand(t, srv.args("escalations", "list", "--output", "table")...)

	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}
	// Exact columns in order: proves the noisy ones (creator, events,
	// related_alerts...) are gone, not just that the wanted ones are present.
	header := strings.Fields(strings.SplitN(res.stdout, "\n", 2)[0])
	want := []string{"id", "title", "status", "priority", "created_at"}
	if !slices.Equal(header, want) {
		t.Errorf("header = %v, want %v", header, want)
	}
}

func TestEscalationsList_JSONKeepsEveryField(t *testing.T) {
	srv := newStubServer(t)
	srv.respond("GET /v2/escalations",
		`{"escalations":[{"id":"01ESC","title":"Queue build-up","escalation_path_id":"01PATH"}],"pagination_meta":{}}`)

	res := runCommand(t, srv.args("escalations", "list")...)

	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &items); err != nil {
		t.Fatal(err)
	}
	// Default columns are a table concern: JSON must not be filtered.
	if _, ok := items[0]["escalation_path_id"]; !ok {
		t.Errorf("JSON output must keep non-default fields, got: %s", res.stdout)
	}
}

func TestSeveritiesList_LimitAppliesWithoutServerPagination(t *testing.T) {
	srv := newStubServer(t)
	srv.respond("GET /v1/severities",
		`{"severities":[{"id":"01A"},{"id":"01B"},{"id":"01C"}]}`)

	res := runCommand(t, srv.args("severities", "list", "--limit", "2")...)

	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Errorf("expected --limit 2 to truncate client-side, got %d items", len(items))
	}
}
