package cmd

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
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

func TestNormalizeIncidentID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// The forms a user has to hand.
		{"reference as printed", "INC-84", "84"},
		{"lowercase reference", "inc-84", "84"},
		{"hash prefix as written in chat", "#INC-84", "84"},
		{"surrounding whitespace", "  INC-84  ", "84"},
		{"bare external id", "84", "84"},
		// Anything we don't recognise goes through untouched for the API to judge.
		{"ULID untouched", "01KZR43MHR9XPQM5MYZ5X6B26Z", "01KZR43MHR9XPQM5MYZ5X6B26Z"},
		{"reference with trailing text is not a reference", "INC-84-typo", "INC-84-typo"},
		{"prefix without digits", "INC-", "INC-"},
		{"hash alone is not a reference", "#", "#"},
		{"signed numbers are not references", "INC-+84", "INC-+84"},
		{"empty stays empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeIncidentID(tt.in); got != tt.want {
				t.Errorf("normalizeIncidentID(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIncidentsShow_AcceptsReference(t *testing.T) {
	srv := newStubServer(t)
	srv.respond("GET /v2/incidents/84", `{"incident":{"id":"01A","reference":"INC-84","name":"one"}}`)

	res := runCommand(t, srv.args("incidents", "show", "INC-84")...)

	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}
	if len(srv.requests) != 1 || srv.requests[0].Path != "/v2/incidents/84" {
		t.Errorf("expected a request to /v2/incidents/84, got %+v", srv.requests)
	}
}

func TestIncidentsCommandsTakingAnID_AcceptReference(t *testing.T) {
	// Enumerated rather than listed, so a new incident subcommand taking <id> is
	// covered the moment it's registered. Forgetting normalizeIncidentID is otherwise
	// invisible: `id := args[0]` is what the other 22 arg-reading commands do.
	//
	// --dry-run prints the request it would send and exits 0, which is enough to prove
	// the reference reached the path and needs no stubbed response per command.
	srv := newStubServer(t)

	for _, sub := range incidentsCmd.Commands() {
		if !strings.Contains(sub.Use, "<id>") {
			continue
		}

		t.Run(sub.Name(), func(t *testing.T) {
			res := runCommand(t, srv.args("incidents", sub.Name(), "#inc-84", "--dry-run")...)

			if res.exit != 0 {
				t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
			}
			if !strings.Contains(res.stderr, "/v2/incidents/84") {
				t.Errorf("expected the reference to resolve to /v2/incidents/84, got: %s", res.stderr)
			}
		})
	}
}

func TestIncidentsList_LimitMatchingPageSizeCostsOneRequest(t *testing.T) {
	// ttyDefaultLimit is aligned with the default --page-size so the common case (an
	// unqualified list on a terminal) is satisfied by the first page. A cap above the
	// page size costs a second round-trip to fetch rows that are then discarded.
	page := func(ids ...string) string {
		items := make([]string, 0, len(ids))
		for _, id := range ids {
			items = append(items, `{"id":"`+id+`"}`)
		}
		return `{"incidents":[` + strings.Join(items, ",") + `],"pagination_meta":{"after":"cursor-1","page_size":25}}`
	}
	// A full page at the default --page-size, deliberately not sized from
	// ttyDefaultLimit: that would make the test pass for any cap.
	const defaultPageSize = 25
	first := make([]string, 0, defaultPageSize)
	for i := range defaultPageSize {
		first = append(first, fmt.Sprintf("01A%02d", i))
	}

	srv := newStubServer(t)
	srv.respond("GET /v2/incidents", page(first...), page("01B00"))

	res := runCommand(t, srv.args("incidents", "list",
		"--limit", strconv.Itoa(ttyDefaultLimit), "--page-size", strconv.Itoa(defaultPageSize))...)

	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}
	if len(srv.requests) != 1 {
		t.Errorf("a limit equal to the page size should be satisfied by one request, got %d", len(srv.requests))
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != ttyDefaultLimit {
		t.Errorf("expected %d items, got %d", ttyDefaultLimit, len(items))
	}
}
