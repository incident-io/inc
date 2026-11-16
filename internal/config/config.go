package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/spf13/viper"
)

const (
	DefaultAPIURL = "https://api.incident.io"
	DefaultAppURL = "https://app.incident.io"
	DefaultOutput = "table"
)

// Config holds the CLI configuration.
type Config struct {
	APIKey string `mapstructure:"api_key"`
	APIURL string `mapstructure:"api_url"`
	// AppURL is the dashboard host, which serves the OAuth authorize and
	// token-exchange endpoints (they are not on the public API host).
	AppURL string `mapstructure:"app_url"`
	Output string `mapstructure:"default_output"`
	// OAuthToken is a browser-login (OAuth) access token, used when no API
	// key is set. OAuthExpiresAt is its RFC3339 expiry. OAuthRefreshToken,
	// when the server issued one, gets a new access token when it expires;
	// each refresh token works once.
	OAuthToken        string `mapstructure:"oauth_token"`
	OAuthExpiresAt    string `mapstructure:"oauth_expires_at"`
	OAuthRefreshToken string `mapstructure:"oauth_refresh_token"`
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
// (client construction, output-format resolution). Save does NOT invalidate
// the cache. The one writer that reads back in the same process, the OAuth
// refresh, reads the file with LoadFile under Lock instead.
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

// LoadOrDefaults is Load that degrades to the defaults instead of an error
// when the config file can't be read. Don't save what it returns: it includes
// environment variables and defaults. Writers start from LoadFile.
func LoadOrDefaults() *Config {
	cfg, err := Load()
	if err != nil {
		return &Config{
			APIURL: DefaultAPIURL,
			AppURL: DefaultAppURL,
			Output: DefaultOutput,
		}
	}
	return cfg
}

func load() (*Config, error) {
	v := viper.New()
	v.SetConfigFile(ConfigFilePath())
	v.SetConfigType("yaml")

	// Defaults
	v.SetDefault("api_url", DefaultAPIURL)
	v.SetDefault("app_url", DefaultAppURL)
	v.SetDefault("default_output", DefaultOutput)

	// Environment variable bindings
	v.SetEnvPrefix("")
	_ = v.BindEnv("api_key", "INCIDENT_API_KEY")
	_ = v.BindEnv("api_url", "INCIDENT_API_URL")
	_ = v.BindEnv("app_url", "INCIDENT_APP_URL")
	_ = v.BindEnv("default_output", "INCIDENT_DEFAULT_OUTPUT")

	// Read config file (ignore if missing)
	_ = v.ReadInConfig()

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}

	// An empty value in the file means unset: fall back to the default so
	// callers never see "".
	if cfg.APIURL == "" {
		cfg.APIURL = DefaultAPIURL
	}
	if cfg.AppURL == "" {
		cfg.AppURL = DefaultAppURL
	}
	if cfg.Output == "" {
		cfg.Output = DefaultOutput
	}
	return cfg, nil
}

// LoadFile reads the config file alone, with no environment variables and no
// defaults, and without the Load cache. A caller that reads, changes and saves
// the file uses it, so values from the environment aren't written into the
// file and changes made by another process since Load aren't lost.
func LoadFile() (*Config, error) {
	v := viper.New()
	v.SetConfigFile(ConfigFilePath())
	v.SetConfigType("yaml")

	// Read config file (ignore if missing)
	_ = v.ReadInConfig()

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Lock takes an exclusive lock shared by every inc process, waiting until it
// is free or ctx is done. Hold it to read, change and save the config without
// another process changing it in between. The returned function releases it.
// The operating system releases the lock if the process dies.
func Lock(ctx context.Context) (unlock func(), err error) {
	dir := configDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}

	lock := flock.New(filepath.Join(dir, "config.lock"), flock.SetPermissions(0o600))
	locked, err := lock.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("locking config: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("locking config: %w", ctx.Err())
	}
	return func() { _ = lock.Unlock() }, nil
}

// Update changes the config file under Lock: it reads the file alone (see
// LoadFile), applies change and saves. Holding the lock means a concurrent
// OAuth refresh can't save over the change. A file that can't be parsed is
// replaced.
func Update(ctx context.Context, change func(cfg *Config)) error {
	unlock, err := Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()

	cfg, err := LoadFile()
	if err != nil {
		cfg = &Config{}
	}
	change(cfg)
	return Save(cfg)
}

// Save writes the config to disk, creating the directory if needed. The file
// holds credentials, so it is kept private (0600). It is written to a temp
// file and renamed into place, so a reader never sees a partly written file.
func Save(cfg *Config) error {
	dir := configDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	v := viper.New()
	v.SetConfigType("yaml")

	// Empty values are omitted entirely, so an unset key disappears from the
	// file and the default applies on the next load.
	for key, value := range map[string]string{
		"api_key":             cfg.APIKey,
		"api_url":             cfg.APIURL,
		"app_url":             cfg.AppURL,
		"default_output":      cfg.Output,
		"oauth_token":         cfg.OAuthToken,
		"oauth_expires_at":    cfg.OAuthExpiresAt,
		"oauth_refresh_token": cfg.OAuthRefreshToken,
	} {
		if value != "" {
			v.Set(key, value)
		}
	}

	// CreateTemp makes the file 0600. The .yml suffix is what viper needs to
	// write it.
	tmp, err := os.CreateTemp(dir, ".config-*.yml")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmpPath) }() // no-op once renamed

	if err := v.WriteConfigAs(tmpPath); err != nil {
		return err
	}
	return os.Rename(tmpPath, ConfigFilePath())
}
