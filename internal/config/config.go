package config

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/viper"
)

const (
	DefaultAPIURL = "https://api.incident.io"
	DefaultOutput = "table"
)

// Config holds the CLI configuration.
type Config struct {
	APIKey string `mapstructure:"api_key"`
	APIURL string `mapstructure:"api_url"`
	Output string `mapstructure:"default_output"`
}

// configDir returns the XDG-compatible config directory.
func configDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "inc")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "inc")
}

// ConfigFilePath returns the path to the config file.
func ConfigFilePath() string {
	return filepath.Join(configDir(), "config.yml")
}

// loadOnce memoizes the config read: commands load config in several places
// (client construction, output-format resolution) and the file cannot change
// mid-invocation. Save does NOT invalidate the cache; no caller reads back
// after saving in the same process.
var loadOnce = sync.OnceValues(load)

// Load returns the config from ~/.config/inc/config.yml with environment
// variables bound. The result is a copy, so callers may mutate it. Flag
// values are NOT resolved here — that happens at the command level via
// Resolve.
func Load() (*Config, error) {
	cfg, err := loadOnce()
	if err != nil {
		return nil, err
	}
	cp := *cfg
	return &cp, nil
}

func load() (*Config, error) {
	v := viper.New()
	v.SetConfigFile(ConfigFilePath())
	v.SetConfigType("yaml")

	// Defaults
	v.SetDefault("api_url", DefaultAPIURL)
	v.SetDefault("default_output", DefaultOutput)

	// Environment variable bindings
	v.SetEnvPrefix("")
	_ = v.BindEnv("api_key", "INCIDENT_API_KEY")
	_ = v.BindEnv("api_url", "INCIDENT_API_URL")
	_ = v.BindEnv("default_output", "INCIDENT_DEFAULT_OUTPUT")

	// Read config file (ignore if missing)
	_ = v.ReadInConfig()

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save writes the config to disk, creating the directory if needed. The file
// holds the API key, so it is kept private (0600) even if it already existed
// with looser permissions.
func Save(cfg *Config) error {
	dir := configDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	path := ConfigFilePath()
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetConfigPermissions(0o600)

	v.Set("api_key", cfg.APIKey)
	v.Set("api_url", cfg.APIURL)
	v.Set("default_output", cfg.Output)

	if err := v.WriteConfig(); err != nil {
		// WriteConfig fails if the file doesn't exist yet (fresh install).
		if err := v.WriteConfigAs(path); err != nil {
			return err
		}
	}
	// Creation modes don't apply to pre-existing files; tighten those too.
	return os.Chmod(path, 0o600)
}
