// Package config loads and saves omega's configuration and resolves its
// on-disk locations (XDG-style, under ~/.config and ~/.local/state).
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/steps"
)

// Approval policies.
const (
	ApproveDangerous = "dangerous" // ask only for risky actions (default)
	ApproveAll       = "all"       // ask before anything that is not read-only
	ApproveNever     = "never"     // never ask (still refuses blocked commands)
)

// Values of model_catalog.
const (
	CatalogLive = "live"
	CatalogOff  = "off"
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
	Provider  string   `toml:"provider"`
	Model     string   `toml:"model,omitempty"`
	Effort    string   `toml:"effort,omitempty"` // Claude models: low|medium|high|xhigh|max
	Approval  string   `toml:"approval"`
	Language  string   `toml:"language"` // en, es, fr, de, pt or auto (system locale)
	SkipSteps []string `toml:"skip_steps,omitempty"`
	Timeout   string   `toml:"command_timeout"`
	// Catalog is "off" to stop omega from downloading the public model
	// catalogue that automatic model selection uses; empty or "live" allows it.
	Catalog   string                    `toml:"model_catalog,omitempty"`
	Providers map[string]ProviderConfig `toml:"providers,omitempty"`
	// DiskBudgets are the most a folder omega can only empty (Claude
	// Desktop's VM, Xcode's DerivedData...) may hold before `omega disk
	// limits enforce` empties it: check id -> size such as "8GB".
	DiskBudgets map[string]string `toml:"disk_budgets,omitempty"`
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

// unknownKeys are the keys of the last file Load read that omega does not
// know (a typo such as "aproval"), as dotted paths.
var (
	unknownMu   sync.Mutex
	unknownKeys []string
)

// UnknownKeys returns the keys of config.toml that the last Load did not
// recognise, such as "aproval" or "providers.openai.modle", sorted. A typo
// there leaves a setting at its default without a word, so `omega config
// check` and `omega doctor` warn about them.
func UnknownKeys() []string {
	unknownMu.Lock()
	defer unknownMu.Unlock()
	return append([]string(nil), unknownKeys...)
}

// Load reads config.toml, falling back to defaults when it does not exist.
func Load() (*Config, error) {
	cfg := Default()
	md, err := toml.DecodeFile(Path(), cfg)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("reading %s: %w", Path(), err)
	}
	var unknown []string
	for _, k := range md.Undecoded() {
		unknown = append(unknown, k.String())
	}
	sort.Strings(unknown)
	unknownMu.Lock()
	unknownKeys = unknown
	unknownMu.Unlock()
	if cfg.Providers == nil {
		cfg.Providers = map[string]ProviderConfig{}
	}
	if cfg.Provider == "" {
		cfg.Provider = "anthropic"
	}
	if cfg.Approval == "" {
		cfg.Approval = ApproveDangerous
	}
	if cfg.Language == "" {
		cfg.Language = "auto"
	}
	if cfg.Timeout == "" {
		cfg.Timeout = "45m"
	}
	// The API takes the effort in lower case; "High" validated and then
	// failed with a 400.
	cfg.Effort = strings.ToLower(strings.TrimSpace(cfg.Effort))
	return cfg, nil
}

// Validate reports a configuration that omega would misread. Load does not
// call it, so `omega config set` can still repair a broken file.
func (c *Config) Validate() error {
	switch c.Approval {
	case ApproveDangerous, ApproveAll, ApproveNever:
	default:
		return fmt.Errorf("%s", i18n.T("config.bad_approval"))
	}
	if c.Language != "auto" && i18n.Normalize(c.Language) == "" {
		return fmt.Errorf("%s", i18n.T("config.bad_language", c.Language))
	}
	d, err := time.ParseDuration(c.Timeout)
	if err != nil || d <= 0 {
		return fmt.Errorf("%s", i18n.T("config.bad_duration", c.Timeout))
	}
	c.Effort = strings.ToLower(strings.TrimSpace(c.Effort))
	if c.Effort != "" {
		switch c.Effort {
		case "low", "medium", "high", "xhigh", "max":
		default:
			return fmt.Errorf("%s", i18n.T("config.bad_effort"))
		}
	}
	switch c.Catalog {
	case "", CatalogLive, CatalogOff:
	default:
		return fmt.Errorf("%s", i18n.T("config.bad_catalog"))
	}
	for _, id := range c.SkipSteps {
		if !steps.Known(id) {
			return fmt.Errorf("%s", i18n.T("config.bad_step", id))
		}
	}
	ids := make([]string, 0, len(c.DiskBudgets))
	for id := range c.DiskBudgets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, err := ParseSize(c.DiskBudgets[id]); err != nil {
			return fmt.Errorf("%s", i18n.T("config.bad_budget", id, c.DiskBudgets[id]))
		}
	}
	return nil
}

var sizeRe = regexp.MustCompile(`(?i)^\s*([0-9]+(?:[.,][0-9]+)?)\s*(b|k|kb|kib|m|mb|mib|g|gb|gib|t|tb|tib)?\s*$`)

// ParseSize reads a size as disk_budgets holds it: 500MB, 8GB, 8 G or
// 1.5gb; a bare number is gigabytes. It reads what assist.ParseSize reads.
func ParseSize(s string) (int64, error) {
	m := sizeRe.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("%q is not a size: write it as 500MB or 8GB", s)
	}
	n, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%q is not a size: write it as 500MB or 8GB", s)
	}
	mult := map[string]float64{"b": 1, "k": 1 << 10, "m": 1 << 20, "g": 1 << 30, "t": 1 << 40}[strings.ToLower(m[2] + "g")[:1]]
	return int64(n * mult), nil
}

// savePath is the file Save replaces. When config.toml is a symbolic link
// (a dotfiles repository, say) it is the file the link points to: renaming
// over the link itself would turn it into an ordinary file and leave the
// repository's copy behind.
func savePath() string {
	path := Path()
	// A chain of links is followed; one that leads nowhere or in a circle
	// stops being followed and the last name found is written.
	for i := 0; i < 16; i++ {
		target, err := os.Readlink(path)
		if err != nil {
			break
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		path = target
	}
	return path
}

// Save writes the configuration to config.toml. The previous file is left in
// place when the new contents are invalid or the write fails.
func (c *Config) Save() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	// The temporary file is made next to the file that is really written,
	// so the rename stays on one disk.
	path := savePath()
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return err
	}
	tmp := f.Name()
	cleanup := true
	defer func() {
		if cleanup {
			os.Remove(tmp)
		}
	}()
	// For this user only: it names the provider's address, the models and
	// the approval policy, which other accounts have no business reading.
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if err := toml.NewEncoder(f).Encode(c); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	cleanup = false
	return nil
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

// scheduled is set by SetScheduled, for a run started with --scheduled.
var scheduled atomic.Bool

// SetScheduled marks this process as a scheduled run, as the flag
// --scheduled of the job `omega schedule install` creates does. It means
// what OMEGA_SCHEDULED=1 in the environment means; the Task Scheduler of
// Windows passes no environment, so the flag is the one every system has.
func SetScheduled(on bool) { scheduled.Store(on) }

// Scheduled reports a scheduled run: --scheduled (SetScheduled) or
// OMEGA_SCHEDULED=1. Nobody is looking when it runs.
func Scheduled() bool {
	return scheduled.Load() || os.Getenv("OMEGA_SCHEDULED") == "1"
}

// trashSteps are the steps that empty the Trash (macOS, Linux) or the
// Recycle Bin (Windows).
var trashSteps = map[string]bool{"trash": true, "recycle-bin": true}

// Skipped reports whether a step is disabled by the user.
func (c *Config) Skipped(id string) bool {
	// A scheduled run never empties the Trash or the Recycle Bin: what
	// omega removes goes there so that it can be put back, and nobody is
	// looking when it runs.
	if trashSteps[id] && Scheduled() {
		return true
	}
	for _, s := range c.SkipSteps {
		if s == id {
			return true
		}
	}
	return false
}
