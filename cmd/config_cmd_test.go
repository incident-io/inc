package cmd

import (
	"testing"

	"github.com/incident-io/inc/internal/config"
)

func TestConfigSetAndUnset_DoNotWriteEnvironmentIntoFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("INCIDENT_API_URL", "https://env.example.com")

	if res := runCommand(t, "config", "set", "default_output", "json"); res.exit != 0 {
		t.Fatalf("config set failed: %+v", res)
	}
	saved, err := config.LoadFile()
	if err != nil {
		t.Fatal(err)
	}
	if *saved != (config.Config{Output: "json"}) {
		t.Errorf("expected only default_output in the file, got %+v", saved)
	}

	if res := runCommand(t, "config", "unset", "default_output"); res.exit != 0 {
		t.Fatalf("config unset failed: %+v", res)
	}
	saved, err = config.LoadFile()
	if err != nil {
		t.Fatal(err)
	}
	if *saved != (config.Config{}) {
		t.Errorf("expected an empty file, got %+v", saved)
	}
}
