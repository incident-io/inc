package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAPI_GetPipedOutputsJSONWithJQ(t *testing.T) {
	srv := newStubServer(t)
	srv.respond("GET /v2/widgets", `{"widgets":[{"id":"01A"},{"id":"01B"}]}`)

	res := runCommand(t, srv.args("api", "GET", "/v2/widgets", "--jq", ".widgets[].id")...)

	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}
	// Piped output resolves to JSON, so --jq applies without --output json.
	if got := strings.Fields(res.stdout); len(got) != 2 || got[0] != `"01A"` || got[1] != `"01B"` {
		t.Errorf("expected jq-filtered ids, got: %q", res.stdout)
	}
}

func TestAPI_PostFieldsBecomeBody(t *testing.T) {
	srv := newStubServer(t)
	srv.respond("POST /v2/incidents", `{"incident":{"id":"01NEW"}}`)

	res := runCommand(t, srv.args("api", "POST", "/v2/incidents", "--field", "name=Outage", "--field", "visibility=public")...)

	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(srv.requests[0].Body), &sent); err != nil {
		t.Fatal(err)
	}
	if sent["name"] != "Outage" || sent["visibility"] != "public" {
		t.Errorf("unexpected body: %s", srv.requests[0].Body)
	}
	// Raw response passes through unenveloped.
	if !strings.Contains(res.stdout, `"incident"`) {
		t.Errorf("expected raw response on stdout, got: %s", res.stdout)
	}
}

func TestAPI_GetFieldsBecomeQuery(t *testing.T) {
	srv := newStubServer(t)
	srv.respond("GET /v2/incidents", `{"incidents":[]}`)

	res := runCommand(t, srv.args("api", "GET", "/v2/incidents", "--field", "status_category[one_of]=live")...)

	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}
	if !strings.Contains(srv.requests[0].Query, "status_category%5Bone_of%5D=live") {
		t.Errorf("expected field as query param, got: %s", srv.requests[0].Query)
	}
}

func TestAPI_PaginateCollectsPages(t *testing.T) {
	srv := newStubServer(t)
	srv.respond("GET /v2/incidents",
		`{"incidents":[{"id":"01A"}],"pagination_meta":{"after":"cur-1"}}`,
		`{"incidents":[{"id":"01B"}],"pagination_meta":{}}`,
	)

	res := runCommand(t, srv.args("api", "GET", "/v2/incidents", "--paginate")...)

	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}
	if len(srv.requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(srv.requests))
	}
	if !strings.Contains(srv.requests[1].Query, "after=cur-1") {
		t.Errorf("second request should carry cursor, got: %s", srv.requests[1].Query)
	}
	// Multiple pages come back as a JSON array of page objects.
	var pages []map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &pages); err != nil {
		t.Fatalf("expected JSON array of pages: %v in %s", err, res.stdout)
	}
	if len(pages) != 2 {
		t.Errorf("expected 2 pages, got %d", len(pages))
	}
}

func TestAPI_PaginateAppliesJQPerPage(t *testing.T) {
	srv := newStubServer(t)
	srv.respond("GET /v2/incidents",
		`{"incidents":[{"id":"01A"}],"pagination_meta":{"after":"cur-1"}}`,
		`{"incidents":[{"id":"01B"}],"pagination_meta":{}}`,
	)

	res := runCommand(t, srv.args("api", "GET", "/v2/incidents", "--paginate", "--jq", ".incidents[].id")...)

	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}
	// The documented envelope-shaped filter must work across pages.
	if got := strings.Fields(res.stdout); len(got) != 2 || got[0] != `"01A"` || got[1] != `"01B"` {
		t.Errorf("expected per-page jq results, got: %q", res.stdout)
	}
}

func TestAPI_QuietSuppressesOutput(t *testing.T) {
	srv := newStubServer(t)
	srv.respond("GET /v2/incidents", `{"incidents":[{"id":"01A"}]}`)

	res := runCommand(t, srv.args("api", "GET", "/v2/incidents", "--quiet")...)

	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}
	if res.stdout != "" {
		t.Errorf("--quiet should suppress stdout, got: %q", res.stdout)
	}
	if len(srv.requests) != 1 {
		t.Errorf("--quiet must still send the request, server saw %d", len(srv.requests))
	}
}

func TestAPI_DryRunPrintsAndSendsNothing(t *testing.T) {
	srv := newStubServer(t)

	res := runCommand(t, srv.args("api", "POST", "/v2/incidents", "--field", "name=Outage", "--dry-run")...)

	if res.exit != 0 {
		t.Fatalf("dry-run should exit 0, got %d, stderr: %s", res.exit, res.stderr)
	}
	if len(srv.requests) != 0 {
		t.Errorf("dry-run must not send requests, server saw %d", len(srv.requests))
	}
	if !strings.Contains(res.stderr, `"method": "POST"`) {
		t.Errorf("expected request preview on stderr, got: %s", res.stderr)
	}
}
