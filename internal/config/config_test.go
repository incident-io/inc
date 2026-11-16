package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestSave_RoundTripsLoginAndLeavesNoTempFiles(t *testing.T) {
	clearEnv(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	in := &Config{OAuthToken: "access", OAuthExpiresAt: "2026-01-01T00:00:00Z", OAuthRefreshToken: "refresh"}
	if err := Save(in); err != nil {
		t.Fatal(err)
	}
	out, err := LoadFile()
	if err != nil {
		t.Fatal(err)
	}
	if *out != *in {
		t.Errorf("expected %+v back, got %+v", in, out)
	}

	entries, err := os.ReadDir(filepath.Dir(ConfigFilePath()))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.yml" {
		t.Errorf("expected only config.yml in the config dir, got %v", entries)
	}
}

func TestLoadFile_IgnoresEnvironmentAndDefaults(t *testing.T) {
	// LoadFile feeds read-modify-save, so nothing that isn't in the file may
	// come back from it and get written into the file.
	writeConfig(t, "oauth_token: from-file\n")
	t.Setenv("INCIDENT_API_KEY", "from-env")
	t.Setenv("INCIDENT_API_URL", "https://env.example.com")

	cfg, err := LoadFile()
	if err != nil {
		t.Fatal(err)
	}
	if *cfg != (Config{OAuthToken: "from-file"}) {
		t.Errorf("expected only the file's values, got %+v", cfg)
	}
}

func TestLock_IsExclusive(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	unlock, err := Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := Lock(ctx); err == nil {
		t.Fatal("expected a second Lock to wait while the first is held")
	}

	unlock()
	unlockAgain, err := Lock(context.Background())
	if err != nil {
		t.Fatalf("expected Lock to succeed once released: %v", err)
	}
	unlockAgain()
}

func TestUpdate_WaitsForLockAndKeepsOtherValues(t *testing.T) {
	clearEnv(t)
	writeConfig(t, "oauth_token: kept\n")

	// A refresh holding the lock must finish before Update reads the file.
	unlock, err := Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := Update(ctx, func(cfg *Config) { cfg.Output = "json" }); err == nil {
		t.Fatal("expected Update to wait while the lock is held")
	}
	unlock()

	if err := Update(context.Background(), func(cfg *Config) { cfg.Output = "json" }); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFile()
	if err != nil {
		t.Fatal(err)
	}
	if *cfg != (Config{OAuthToken: "kept", Output: "json"}) {
		t.Errorf("expected the change on top of the file's values, got %+v", cfg)
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
