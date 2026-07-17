package cmd

import (
	"encoding/json"
	"testing"
)

func TestDescribe_OutputsFullSchema(t *testing.T) {
	res := runCommand(t, "describe")

	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}
	schema := mustJSON(t, res.stdout)
	top, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("expected a JSON object, got %T", schema)
	}
	if top["name"] != "inc" {
		t.Errorf("expected the root command node, got name: %v", top["name"])
	}
	subcommands, _ := top["subcommands"].([]any)
	found := false
	for _, c := range subcommands {
		if m, ok := c.(map[string]any); ok && m["name"] == "incidents" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected an incidents entry in the schema's subcommands")
	}
}

func TestDescribe_DotPathSelectsOneCommand(t *testing.T) {
	res := runCommand(t, "describe", "incidents.list")

	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr: %s", res.exit, res.stderr)
	}
	var node map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &node); err != nil {
		t.Fatal(err)
	}
	if node["name"] != "list" {
		t.Errorf("expected the list command node, got: %s", res.stdout)
	}
}
