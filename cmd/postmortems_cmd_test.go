package cmd

import (
	"strings"
	"testing"
)

func TestPostmortemsContent_OutputFlags(t *testing.T) {
	for _, tt := range []struct {
		name      string
		flags     []string
		want      string
		wantError bool
	}{
		{name: "jq implies JSON", flags: []string{"--jq", ".markdown"}, want: `"# Outage report"`},
		{name: "fields", flags: []string{"--output", "json", "--fields", "markdown"}, want: `{"markdown":"# Outage report"}`},
		{name: "invalid jq", flags: []string{"--jq", "invalid("}, wantError: true},
		{name: "markdown", flags: []string{"--output", "table"}, want: "# Outage report"},
		{name: "quiet", flags: []string{"--jq", ".markdown", "--quiet"}, want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := newStubServer(t)
			srv.respond("GET /v1/postmortem_documents/doc-id/content", `{"markdown":"# Outage report","extra":"unselected"}`)
			args := append([]string{"post-mortems", "content", "doc-id"}, tt.flags...)
			res := runCommand(t, srv.args(args...)...)
			if tt.wantError {
				if res.exit == 0 {
					t.Fatalf("expected invalid jq to fail, stdout: %s", res.stdout)
				}
				return
			}
			if res.exit != 0 {
				t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
			}
			if tt.name == "fields" {
				got := mustJSON(t, res.stdout).(map[string]any)
				if len(got) != 1 || got["markdown"] != "# Outage report" {
					t.Fatalf("unexpected fields: %v", got)
				}
			} else if strings.TrimSpace(res.stdout) != tt.want {
				t.Fatalf("stdout = %q, want %q", res.stdout, tt.want)
			}
		})
	}
}
