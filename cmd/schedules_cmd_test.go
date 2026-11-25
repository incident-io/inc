package cmd

import (
	"net/url"
	"testing"
)

func TestSchedulesEntries_FollowsPagination(t *testing.T) {
	srv := newStubServer(t)
	srv.respond("GET /v2/schedule_entries",
		`{"schedule_entries":{"final":[{"entry_id":"first"}],"overrides":[],"scheduled":[{"entry_id":"s1"}]},"pagination_meta":{"after":"opaque+cursor=","page_size":1}}`,
		`{"schedule_entries":{"final":[{"entry_id":"second"}],"overrides":[{"entry_id":"o2"}],"scheduled":[]},"pagination_meta":{"page_size":1}}`,
	)

	res := runCommand(t, srv.args("schedules", "entries", "sched-id",
		"--from", "2026-01-01T00:00:00Z", "--until", "2026-02-01T00:00:00Z", "--output", "json")...)
	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}

	got := mustJSON(t, res.stdout).(map[string]any)
	for key, want := range map[string][]string{"final": {"first", "second"}, "overrides": {"o2"}, "scheduled": {"s1"}} {
		entries, _ := got[key].([]any)
		if len(entries) != len(want) {
			t.Fatalf("%s: got %v, want %v", key, got[key], want)
		}
		for i, id := range want {
			if entries[i].(map[string]any)["entry_id"] != id {
				t.Fatalf("%s: got %v, want %v", key, got[key], want)
			}
		}
	}

	if len(srv.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(srv.requests))
	}
	first, err := url.ParseQuery(srv.requests[0].Query)
	if err != nil {
		t.Fatal(err)
	}
	second, err := url.ParseQuery(srv.requests[1].Query)
	if err != nil {
		t.Fatal(err)
	}
	if got := second.Get("entry_window_start"); got != "opaque+cursor=" {
		t.Fatalf("entry_window_start = %q, want the cursor unchanged", got)
	}
	if got, want := second.Get("entry_window_end"), first.Get("entry_window_end"); got != want {
		t.Fatalf("entry_window_end = %q, want %q", got, want)
	}
}
