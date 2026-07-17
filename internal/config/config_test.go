package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfig puts a config file where load() will find it and points
// XDG_CONFIG_HOME at it.
func writeConfig(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "inc"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "inc", "config.yml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// clearEnv blanks the bound environment variables so developer shells (mise
// auto-loads .env) can't leak into assertions.
func clearEnv(t *testing.T) {
	t.Helper()
	t.Setenv("INCIDENT_API_KEY", "")
	t.Setenv("INCIDENT_API_URL", "")
	t.Setenv("INCIDENT_DEFAULT_OUTPUT", "")
}

// Tests exercise the unexported load() directly: the exported Load() is
// memoized per process, so it can only observe one environment per test
// binary. Load()'s own copy semantics get one dedicated test.

func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // no config file

	cfg, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIURL != DefaultAPIURL {
		t.Errorf("expected default API URL, got %q", cfg.APIURL)
	}
	if cfg.Output != DefaultOutput {
		t.Errorf("expected default output %q, got %q", DefaultOutput, cfg.Output)
	}
	if cfg.APIKey != "" {
		t.Errorf("expected empty API key, got %q", cfg.APIKey)
	}
}

func TestLoad_ReadsConfigFile(t *testing.T) {
	clearEnv(t)
	writeConfig(t, "api_key: inc_test123\napi_url: https://example.com\ndefault_output: json\n")

	cfg, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "inc_test123" || cfg.APIURL != "https://example.com" || cfg.Output != "json" {
		t.Errorf("config file values not loaded: %+v", cfg)
	}
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	writeConfig(t, "api_key: from-file\ndefault_output: table\n")
	t.Setenv("INCIDENT_API_KEY", "from-env")
	t.Setenv("INCIDENT_API_URL", "")
	t.Setenv("INCIDENT_DEFAULT_OUTPUT", "json")

	cfg, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "from-env" {
		t.Errorf("expected INCIDENT_API_KEY to win, got %q", cfg.APIKey)
	}
	if cfg.Output != "json" {
		t.Errorf("expected INCIDENT_DEFAULT_OUTPUT to win, got %q", cfg.Output)
	}
}

func TestSave_CreatesPrivateFile(t *testing.T) {
	clearEnv(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := Save(&Config{APIKey: "inc_secret"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(ConfigFilePath())
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config file holds the API key and must be 0600, got %o", perm)
	}
}

func TestSave_TightensExistingFilePermissions(t *testing.T) {
	clearEnv(t)
	writeConfig(t, "api_key: old\n")
	if err := os.Chmod(ConfigFilePath(), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Save(&Config{APIKey: "inc_secret"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(ConfigFilePath())
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("pre-existing config file must be tightened to 0600, got %o", perm)
	}
}

func TestLoad_ReturnsIndependentCopies(t *testing.T) {
	clearEnv(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	first, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	first.APIKey = "mutated"

	second, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if second.APIKey == "mutated" {
		t.Error("Load must return a copy; a caller's mutation leaked into the cache")
	}
}
