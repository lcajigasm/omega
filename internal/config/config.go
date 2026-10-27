// Package config loads and saves omega's configuration and resolves its
// on-disk locations (XDG-style, under ~/.config and ~/.local/state).
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Approval policies.
const (
	ApproveDangerous = "dangerous" // ask only for risky actions (default)
	ApproveAll       = "all"       // ask before anything that is not read-only
	ApproveNever     = "never"     // never ask (still refuses blocked commands)
)

// ProviderConfig holds per-provider overrides.
type ProviderConfig struct {
	Model      string `toml:"model,omitempty"`       // fixed model; empty or "auto" picks per task
	FastModel  string `toml:"fast_model,omitempty"`  // override for routine work in auto mode
	SmartModel string `toml:"smart_model,omitempty"` // override for failures and fix tasks in auto mode
	BaseURL    string `toml:"base_url,omitempty"`
}

// Config is the user configuration stored in config.toml.
type Config struct {
	Provider  string                    `toml:"provider"`
	Model     string                    `toml:"model,omitempty"`
	Effort    string                    `toml:"effort,omitempty"` // Claude models: low|medium|high|xhigh|max
	Approval  string                    `toml:"approval"`
	Language  string                    `toml:"language"` // en, es, fr, de, pt or auto (system locale)
	SkipSteps []string                  `toml:"skip_steps,omitempty"`
	Timeout   string                    `toml:"command_timeout"`
	Providers map[string]ProviderConfig `toml:"providers,omitempty"`
}

// Default returns the configuration used when no file exists yet.
func Default() *Config {
	return &Config{
		Provider:  "anthropic",
		Approval:  ApproveDangerous,
		Language:  "auto",
		Timeout:   "45m",
		Providers: map[string]ProviderConfig{},
	}
}

// Dir is the configuration directory (~/.config/omega).
func Dir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "omega")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "omega")
}

// StateDir is where run logs are kept (~/.local/state/omega).
func StateDir() string {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "omega")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "omega")
}

// Path is the config file location.
func Path() string { return filepath.Join(Dir(), "config.toml") }

// NotesPath is the file where the agent stores what it learned about this machine.
func NotesPath() string { return filepath.Join(Dir(), "notes.md") }

// Load reads config.toml, falling back to defaults when it does not exist.
func Load() (*Config, error) {
	cfg := Default()
	_, err := toml.DecodeFile(Path(), cfg)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("reading %s: %w", Path(), err)
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]ProviderConfig{}
	}
	if cfg.Approval == "" {
		cfg.Approval = ApproveDangerous
	}
	return cfg, nil
}

// Save writes the configuration to config.toml.
func (c *Config) Save() error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(Path(), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(c)
}

// ModelFor returns the model to use for a provider: the explicit top-level
// model when it targets the active provider, else the provider override.
func (c *Config) ModelFor(provider string) string {
	if provider == c.Provider && c.Model != "" {
		return c.Model
	}
	return c.Providers[provider].Model
}

// BaseURLFor returns a base URL override for the provider, if any.
func (c *Config) BaseURLFor(provider string) string {
	return c.Providers[provider].BaseURL
}

// Skipped reports whether a step is disabled by the user.
func (c *Config) Skipped(id string) bool {
	for _, s := range c.SkipSteps {
		if s == id {
			return true
		}
	}
	return false
}
