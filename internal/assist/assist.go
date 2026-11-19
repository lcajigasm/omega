// Package assist holds omega's configuration assistants: for each AI tool and
// developer tool people use every day (Claude Desktop and Claude Code,
// ChatGPT and Codex, Grok, Cursor, Docker, Homebrew...) what omega checks in
// its own configuration files and can set there, and how much disk it takes.
//
// The disk limits are the checks marked Disk. Where the tool has a setting
// that bounds what it keeps (Claude Code's transcripts, Docker's build cache
// and logs, Codex's history, Homebrew's downloads) omega writes that setting
// in the tool's own file, after a copy of the file is kept. Where it has none
// (Claude Desktop's VM image, the caches of Electron apps, Xcode's
// DerivedData) the limit is a budget omega keeps: `omega disk limits enforce`
// empties the folder once it holds more, and only while the app is closed.
// Old versions an updater leaves behind are a cleanup: nothing is lost.
//
// Nothing here calls a model or leaves the machine.
package assist

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
)

// Kinds of check.
const (
	// KindSetting is a value in the tool's own configuration file.
	KindSetting = "setting"
	// KindBudget is a folder the tool never trims: omega empties it when it
	// holds more than the budget the user set.
	KindBudget = "budget"
	// KindCleanup frees space without losing anything: old versions and
	// downloaded installers an updater keeps.
	KindCleanup = "cleanup"
	// KindAction is a one-off change: register omega's MCP server, keep
	// secrets out of reach.
	KindAction = "action"
	// KindInfo is measured and shown, never changed.
	KindInfo = "info"
)

// Check is one thing an assistant looks at.
type Check struct {
	ID          string `json:"id"`   // tool.name, unique
	Tool        string `json:"tool"` // tool id
	Title       string `json:"title"`
	Why         string `json:"why"`
	Kind        string `json:"kind"`
	Disk        bool   `json:"disk"`           // part of the disk limits
	Size        int64  `json:"size,omitempty"` // bytes the check is about
	SizeText    string `json:"size_text,omitempty"`
	Free        int64  `json:"free,omitempty"` // bytes a cleanup or budget would free now
	Current     string `json:"current"`        // what is set now, in words
	Recommended string `json:"recommended,omitempty"`
	Unit        string `json:"unit,omitempty"` // what a value is given in: days, GB, MB...
	OK          bool   `json:"ok"`             // nothing to do
	File        string `json:"file,omitempty"` // the file a change is written to
	Note        string `json:"note,omitempty"`
	Blocked     string `json:"blocked,omitempty"` // why it cannot be changed right now

	set   func(value string) (string, error)
	clean func() (string, error)
}

// Settable reports whether Apply can change the check.
func (c *Check) Settable() bool {
	return c.Kind != KindInfo && (c.set != nil || c.clean != nil)
}

// Tool is one application or command-line tool with its checks.
type Tool struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Kind      string  `json:"kind"` // app, cli or both
	Installed bool    `json:"installed"`
	Running   bool    `json:"running"`
	Version   string  `json:"version,omitempty"`
	Where     string  `json:"where,omitempty"` // its data folder or config file
	DataSize  int64   `json:"data_size,omitempty"`
	DataText  string  `json:"data_text,omitempty"`
	Checks    []Check `json:"checks"`
	Pending   int     `json:"pending"` // checks with something to do
	About     string  `json:"about"`
}

// Report is every known tool, installed or not.
type Report struct {
	Tools []Tool `json:"tools"`
	// Disk is the checks marked Disk of the installed tools, largest first.
	Disk     []Check `json:"disk"`
	DiskSize int64   `json:"disk_size"`
	DiskText string  `json:"disk_text"`
	Freeable int64   `json:"freeable"`
	FreeText string  `json:"free_text"`
}

// A tool definition builds the tool as this machine has it.
type definition struct {
	id    string
	build func(e *env) Tool
}

var definitions []definition

func register(id string, build func(e *env) Tool) {
	definitions = append(definitions, definition{id, build})
}

// IDs lists the known tools in the order they are shown.
func IDs() []string {
	var out []string
	for _, d := range definitions {
		out = append(out, d.id)
	}
	return out
}

// Scan builds every tool. The tools are measured at once: walking Claude's
// VM image and Cursor's storage takes a moment each.
func Scan() Report {
	e := newEnv()
	tools := make([]Tool, len(definitions))
	var wg sync.WaitGroup
	for i, d := range definitions {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := d.build(e)
			t.ID = d.id
			finish(&t)
			tools[i] = t
		}()
	}
	wg.Wait()
	r := Report{Tools: tools}
	for _, t := range tools {
		if !t.Installed {
			continue
		}
		for _, c := range t.Checks {
			if c.Disk {
				r.Disk = append(r.Disk, c)
				r.DiskSize += c.Size
				r.Freeable += c.Free
			}
		}
	}
	sort.SliceStable(r.Disk, func(i, j int) bool { return r.Disk[i].Size > r.Disk[j].Size })
	r.DiskText = Human(r.DiskSize)
	r.FreeText = Human(r.Freeable)
	return r
}

func finish(t *Tool) {
	if t.DataSize > 0 {
		t.DataText = Human(t.DataSize)
	}
	if !t.Installed {
		t.Checks = nil
		return
	}
	for i := range t.Checks {
		c := &t.Checks[i]
		c.Tool = t.ID
		if c.Size > 0 {
			c.SizeText = Human(c.Size)
		}
		if t.Running && c.Blocked == "" && c.needsClosed() {
			c.Blocked = fmt.Sprintf("%s is open: quit it first", t.Name)
		}
		if !c.OK && c.Kind != KindInfo {
			t.Pending++
		}
	}
}

// needsClosed is true for changes an open app would undo or trip over:
// emptying its folders and writing a file it rewrites.
func (c *Check) needsClosed() bool {
	return c.Kind == KindBudget || c.Kind == KindCleanup && strings.Contains(c.ID, "cache")
}

// Find returns a tool by id or name, case and spaces ignored.
func (r Report) Find(name string) (*Tool, bool) {
	key := normalize(name)
	for i := range r.Tools {
		t := &r.Tools[i]
		if normalize(t.ID) == key || normalize(t.Name) == key || slices.Contains(aliases[t.ID], key) {
			return t, true
		}
	}
	return nil, false
}

// aliases are other names people use for a tool.
var aliases = map[string][]string{
	"claude-code":    {"claude", "claudecode", "claudecli"},
	"claude-desktop": {"claudeapp", "claudedesktop"},
	"codex":          {"chatgpt", "openai", "chatgptapp", "codexcli", "chatgptcodex"},
	"grok":           {"grokbot", "xai", "grokcli"},
	"docker":         {"dockerdesktop"},
	"homebrew":       {"brew"},
	"vscode":         {"code", "visualstudiocode"},
	"gemini-cli":     {"gemini"},
	"xcode":          {"deriveddata"},
}

func normalize(s string) string {
	s = strings.ToLower(s)
	return strings.NewReplacer(" ", "", "-", "", "_", "", "/", "", ".", "").Replace(s)
}

// Check returns a check by id.
func (r Report) Check(id string) (*Check, bool) {
	for i := range r.Tools {
		for j := range r.Tools[i].Checks {
			if r.Tools[i].Checks[j].ID == id {
				return &r.Tools[i].Checks[j], true
			}
		}
	}
	return nil, false
}

// ErrBlocked is returned for a check that cannot be changed right now.
var ErrBlocked = errors.New("blocked")

// Apply makes a check's change: value is the setting or the budget to write,
// "" for the recommended one. A cleanup ignores it.
func Apply(id, value string) (string, error) {
	r := Scan()
	c, ok := r.Check(id)
	if !ok {
		return "", fmt.Errorf("no check %q here: `omega assist` lists them", id)
	}
	return c.Apply(value)
}

// Apply makes the change on a check that was just scanned.
func (c *Check) Apply(value string) (string, error) {
	if c.Blocked != "" {
		return "", fmt.Errorf("%w: %s", ErrBlocked, c.Blocked)
	}
	if !c.Settable() {
		return "", fmt.Errorf("%s is measured only: omega cannot change it", c.ID)
	}
	if c.Kind == KindCleanup || (c.Kind == KindBudget && value == "now") {
		return c.clean()
	}
	if value == "" {
		value = c.Recommended
	}
	if value == "" {
		return "", fmt.Errorf("%s needs a value (%s)", c.ID, c.Unit)
	}
	return c.set(value)
}

// Enforce applies the disk limits omega keeps itself: the cleanups, and the
// budgets that are exceeded. With dry set it only says what it would do.
// A folder of an open app is left alone and said.
func Enforce(dry bool) ([]string, error) {
	r := Scan()
	var out []string
	var errs []error
	for _, c := range r.Disk {
		due := c.Kind == KindCleanup && !c.OK && c.Free > 0 ||
			c.Kind == KindBudget && c.Free > 0
		if !due {
			continue
		}
		if c.Blocked != "" {
			out = append(out, fmt.Sprintf("skip %s: %s", c.ID, c.Blocked))
			continue
		}
		if dry {
			out = append(out, fmt.Sprintf("would free %s: %s", Human(c.Free), c.Title))
			continue
		}
		msg, err := c.clean()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", c.ID, err))
			continue
		}
		out = append(out, msg)
	}
	return out, errors.Join(errs...)
}

// ---------- the machine ----------

// env is what the definitions read of the machine. Tests replace it.
type env struct {
	home    string
	goos    string
	apps    string // where macOS apps are installed
	look    func(string) (string, error)
	running func(pattern string) bool
	omega   string // the omega executable other tools are pointed at
	budgets map[string]string
}

var newEnv = func() *env {
	home, _ := os.UserHomeDir()
	exe := omegaPath()
	return &env{
		home:    home,
		goos:    runtime.GOOS,
		apps:    "/Applications",
		look:    lookPath,
		running: processRunning,
		omega:   exe,
		budgets: loadBudgets(),
	}
}

func (e *env) path(parts ...string) string {
	return filepath.Join(append([]string{e.home}, parts...)...)
}

// appSupport is the folder an app keeps its data in: Application Support
// on macOS, ~/.config on Linux, %APPDATA% on Windows.
func (e *env) appSupport(name string) string {
	switch e.goos {
	case "darwin":
		return e.path("Library", "Application Support", name)
	case "windows":
		if d := os.Getenv("APPDATA"); d != "" && e.home == homeDir() {
			return filepath.Join(d, name)
		}
		return e.path("AppData", "Roaming", name)
	default:
		return e.path(".config", name)
	}
}

// app returns the bundle of a macOS app when it is installed.
func (e *env) app(name string) (string, bool) {
	if e.goos != "darwin" {
		return "", false
	}
	for _, dir := range []string{e.apps, e.path("Applications")} {
		p := filepath.Join(dir, name+".app")
		if isDir(p) {
			return p, true
		}
	}
	return "", false
}

func (e *env) appRunning(bundle string) bool {
	return bundle != "" && e.running(filepath.Join(bundle, "Contents", "MacOS")+"/")
}

func (e *env) has(cmd string) (string, bool) {
	p, err := e.look(cmd)
	return p, err == nil
}

func homeDir() string { h, _ := os.UserHomeDir(); return h }

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func isDir(p string) bool { fi, err := os.Stat(p); return err == nil && fi.IsDir() }
