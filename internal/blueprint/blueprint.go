// Package blueprint writes down what is installed on a machine (Homebrew
// formulae and casks, global npm packages, pipx packages, the developer tools
// omega knows, and omega's own settings) and installs the packages again on
// another one. The developer tools are listed for reference only: bootstrap
// does not install them.
package blueprint

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/devtools"
	"github.com/lcajigasm/omega/internal/runner"
)

var goos = runtime.GOOS

// Seams the tests replace.
var (
	has = runner.Has
	run = runner.Run
)

// listTimeout is how long one package manager gets to list what it has
// installed. brew and npm take well over five seconds on a cold start.
const listTimeout = 2 * time.Minute

// SystemBlueprint holds the complete snapshot of the system configuration and tools.
type SystemBlueprint struct {
	Version      string         `json:"version"`
	ExportDate   time.Time      `json:"export_date"`
	Platform     string         `json:"platform"`
	Hostname     string         `json:"hostname"`
	Homebrew     []string       `json:"homebrew,omitempty"`
	Casks        []string       `json:"casks,omitempty"`
	NpmGlobal    []string       `json:"npm_global,omitempty"`
	PipxPackages []string       `json:"pipx_packages,omitempty"`
	DevTools     []string       `json:"dev_tools,omitempty"`
	Config       *config.Config `json:"config,omitempty"`
	// Unlisted names what could not be listed, so nobody takes an incomplete
	// file for the whole machine.
	Unlisted []string          `json:"unlisted,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// lines returns the non-empty lines of a command's output.
func lines(output string) []string {
	var out []string
	for _, line := range strings.Split(output, "\n") {
		if l := strings.TrimSpace(line); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// noStderr drops a command's stderr in the shell omega runs commands with.
func noStderr() string {
	if goos == "windows" {
		return " 2>$null"
	}
	return " 2>/dev/null"
}

// npmName is the package a line of `npm list -g --parseable` names: what
// follows the last node_modules directory, so a scoped package keeps its
// scope (@scope/pkg). The first line, the directory itself, names nothing.
func npmName(path string) string {
	path = strings.ReplaceAll(strings.TrimSpace(path), `\`, "/")
	i := strings.LastIndex(path, "/node_modules/")
	if i < 0 {
		return ""
	}
	name := path[i+len("/node_modules/"):]
	if name == "npm" {
		return "" // npm comes with Node
	}
	return name
}

// Export generates a SystemBlueprint and writes it to path. A package
// manager that fails or takes too long is named in Unlisted.
func Export(ctx context.Context, path string) (*SystemBlueprint, error) {
	hostname, _ := os.Hostname()
	bp := &SystemBlueprint{
		Version:    "1.0",
		ExportDate: time.Now(),
		Platform:   goos,
		Hostname:   hostname,
		Metadata:   make(map[string]string),
	}
	// list runs one inventory command and says so when it fails.
	list := func(what, command string) []string {
		// Warnings go to stderr and are not package names.
		res := run(ctx, command+noStderr(), listTimeout, nil)
		if res.ExitCode != 0 {
			reason := fmt.Sprintf("exit %d", res.ExitCode)
			if res.TimedOut {
				reason = "timed out"
			}
			bp.Unlisted = append(bp.Unlisted, fmt.Sprintf("%s (%s: %s)", what, command, reason))
			return nil
		}
		return lines(res.Output)
	}

	if has("brew") {
		bp.Homebrew = list("Homebrew formulae", "brew leaves")
		bp.Casks = list("Homebrew casks", "brew list --cask")
	}
	if has("npm") {
		for _, line := range list("global npm packages", "npm list -g --depth=0 --parseable") {
			if name := npmName(line); name != "" {
				bp.NpmGlobal = append(bp.NpmGlobal, name)
			}
		}
	}
	if has("pipx") {
		for _, line := range list("pipx packages", "pipx list --short") {
			if f := strings.Fields(line); len(f) > 0 {
				bp.PipxPackages = append(bp.PipxPackages, f[0])
			}
		}
	}

	// The developer tools omega knows and finds here, for reference.
	installed, _ := devtools.Detect()
	for _, t := range installed {
		bp.DevTools = append(bp.DevTools, t.ID)
	}

	if cfg, err := config.Load(); err == nil {
		bp.Config = cfg
	} else {
		bp.Unlisted = append(bp.Unlisted, fmt.Sprintf("omega configuration (%v)", err))
	}

	if path != "" {
		data, err := json.MarshalIndent(bp, "", "  ")
		if err != nil {
			return nil, err
		}
		// The file names everything installed here and holds the settings:
		// it is for its owner only.
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return nil, err
		}
		if err := os.Chmod(path, 0o600); err != nil && goos != "windows" {
			return nil, err
		}
	}
	return bp, nil
}

// packageName is what a package may be called. The names end up on a command
// line, so nothing a shell reads as syntax is allowed.
var packageName = regexp.MustCompile(`^[A-Za-z0-9@/._+-]+$`)

// ValidName reports whether a name from a blueprint is a package name and
// not an option or a piece of shell.
func ValidName(name string) bool {
	return packageName.MatchString(name) && !strings.HasPrefix(name, "-")
}

// Load reads a blueprint file and refuses it when any package name in it is
// not a plain name: a blueprint is a file someone else may have written.
func Load(path string) (*SystemBlueprint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bp SystemBlueprint
	if err := json.Unmarshal(data, &bp); err != nil {
		return nil, fmt.Errorf("parsing blueprint: %w", err)
	}
	for _, list := range []struct {
		field string
		names []string
	}{{"homebrew", bp.Homebrew}, {"casks", bp.Casks}, {"npm_global", bp.NpmGlobal}, {"pipx_packages", bp.PipxPackages}} {
		for _, name := range list.names {
			if !ValidName(name) {
				return nil, fmt.Errorf("%s: %q in %q is not a package name; nothing was installed", path, name, list.field)
			}
		}
	}
	return &bp, nil
}

// Action is one command a bootstrap runs.
type Action struct {
	What    string // what it installs, for the plan
	Command string
	Timeout time.Duration
}

// quote puts names on a command line. Load has already refused anything but
// plain names; the quotes keep them plain in PowerShell too, where an
// unquoted @scope means something else.
func quote(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = "'" + n + "'"
	}
	return strings.Join(q, " ")
}

// Plan lists the commands that install the blueprint's packages here, and
// what is left out because its package manager is not installed.
func (bp *SystemBlueprint) Plan() (actions []Action, skipped []string) {
	add := func(tool, what, command string, names []string, timeout time.Duration) {
		if len(names) == 0 {
			return
		}
		if !has(tool) {
			skipped = append(skipped, fmt.Sprintf("%s (%s is not installed)", what, tool))
			return
		}
		actions = append(actions, Action{What: what, Command: command + " " + quote(names), Timeout: timeout})
	}
	count := func(names []string, what string) string { return fmt.Sprintf("%d %s", len(names), what) }
	add("brew", count(bp.Homebrew, "Homebrew formulae"), "brew install", bp.Homebrew, 30*time.Minute)
	add("brew", count(bp.Casks, "Homebrew casks"), "brew install --cask", bp.Casks, 30*time.Minute)
	add("npm", count(bp.NpmGlobal, "global npm packages"), "npm install -g", bp.NpmGlobal, 15*time.Minute)
	for _, pkg := range bp.PipxPackages {
		// pipx installs one package per call.
		add("pipx", "pipx package "+pkg, "pipx install", []string{pkg}, 5*time.Minute)
	}
	return actions, skipped
}

// Apply runs the actions in order and returns the ones that failed. A
// failure does not stop the ones after it.
func Apply(ctx context.Context, actions []Action, logFn func(string)) (failed []Action) {
	if logFn == nil {
		logFn = func(string) {}
	}
	for _, a := range actions {
		logFn("$ " + a.Command)
		res := run(ctx, a.Command, a.Timeout, logFn)
		if res.ExitCode != 0 {
			logFn(fmt.Sprintf("✗ %s failed (exit %d)", a.What, res.ExitCode))
			failed = append(failed, a)
		}
	}
	return failed
}

// RestoredConfig is the configuration a bootstrap would save: the blueprint's
// settings on top of what this machine has. Two settings never come from a
// file: approval, which decides what runs without asking, and a provider's
// base_url, which decides where the API key and the conversation are sent.
// Both keep the value this machine has. kept names them when the blueprint
// asked for something else.
func (bp *SystemBlueprint) RestoredConfig(current *config.Config) (cfg *config.Config, kept []string) {
	if bp.Config == nil {
		return nil, nil
	}
	c := *bp.Config
	if c.Approval != current.Approval {
		kept = append(kept, fmt.Sprintf("approval stays %q (the blueprint has %q)", current.Approval, c.Approval))
	}
	c.Approval = current.Approval
	// A provider only this machine has stays as it is.
	c.Providers = map[string]config.ProviderConfig{}
	for name, pc := range current.Providers {
		c.Providers[name] = pc
	}
	for name, pc := range bp.Config.Providers {
		mine := current.Providers[name].BaseURL
		if pc.BaseURL != mine {
			kept = append(kept, fmt.Sprintf("providers.%s.base_url is not restored (the blueprint has %q)", name, pc.BaseURL))
		}
		pc.BaseURL = mine
		c.Providers[name] = pc
	}
	return &c, kept
}

// SaveConfig replaces config.toml with cfg. The file that was there is
// copied to config.toml.bak first; backup is its path, or "" when there was
// no file.
func SaveConfig(cfg *config.Config) (backup string, err error) {
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	if old, readErr := os.ReadFile(config.Path()); readErr == nil {
		backup = config.Path() + ".bak"
		if err := os.WriteFile(backup, old, 0o600); err != nil {
			return "", fmt.Errorf("backing up the configuration: %w", err)
		}
	}
	return backup, cfg.Save()
}
