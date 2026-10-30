// Package macos reads and changes what runs on a Mac outside the apps the user
// opens: items that open at login, jobs allowed in the background, menu bar
// apps, the Dock and global keyboard shortcuts. Every change is a shell
// command with its inverse, recorded in a journal so it can be undone.
package macos

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"howett.net/plist"

	"github.com/lcajigasm/omega/internal/config"
)

// Output runs a read-only command and returns its standard output. Tests
// replace it with canned output.
var Output = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// Home is the user's home directory; tests point it at a fixture tree.
var Home = func() string {
	h, _ := os.UserHomeDir()
	return h
}()

func uid() string { return strconv.Itoa(os.Getuid()) }

// Change is one reversible action on the system.
type Change struct {
	ID      string `json:"id"`               // stable id the agent and the cards use, e.g. "disable:com.google.keystone.agent"
	Section string `json:"section"`          // login, background, menubar, dock, shortcuts
	Title   string `json:"title"`            // what it does, in the interface language
	Command string `json:"command"`          // shell command that makes the change
	Undo    string `json:"undo"`             // shell command that reverts it ("" when it cannot be undone)
	Manual  string `json:"manual,omitempty"` // what the user must do by hand if the command is not enough
}

// Sudo reports whether the change needs administrator access.
func (c Change) Sudo() bool { return strings.Contains(c.Command, "sudo ") }

// Entry is a change that was applied, as stored in the journal.
type Entry struct {
	Time time.Time `json:"time"`
	Change
	Undone bool `json:"undone,omitempty"`
}

// JournalPath is where applied changes are recorded.
func JournalPath() string { return filepath.Join(config.StateDir(), "mac-journal.jsonl") }

// Record appends an applied change to the journal.
func Record(c Change) error {
	if err := os.MkdirAll(config.StateDir(), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(JournalPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if c.Section == "login" || c.Section == "background" {
		ForgetBTM()
	}
	b, _ := json.Marshal(Entry{Time: time.Now(), Change: c})
	_, err = f.Write(append(b, '\n'))
	return err
}

// Journal returns the recorded changes, newest first.
func Journal() []Entry {
	f, err := os.Open(JournalPath())
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			out = append([]Entry{e}, out...)
		}
	}
	return out
}

// MarkUndone flags journal entries as reverted so they are not offered again.
func MarkUndone(times map[time.Time]bool) error {
	entries := Journal()
	var b strings.Builder
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if times[e.Time] {
			e.Undone = true
		}
		line, _ := json.Marshal(e)
		b.Write(line)
		b.WriteByte('\n')
	}
	return os.WriteFile(JournalPath(), []byte(b.String()), 0o644)
}

// readPlist decodes a plist file (XML or binary) into a generic map.
func readPlist(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return decodePlist(data)
}

func decodePlist(data []byte) map[string]any {
	var m map[string]any
	if _, err := plist.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}

// exportDomain reads a preferences domain through cfprefsd, which sees
// changes that are not flushed to disk yet.
func exportDomain(domain string) map[string]any {
	out, err := Output("defaults", "export", domain, "-")
	if err != nil {
		return nil
	}
	return decodePlist(out)
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "1" || strings.EqualFold(x, "yes") || strings.EqualFold(x, "true")
	case uint64, int64, float64:
		return asInt(x) != 0
	}
	return false
}

func asInt(v any) int {
	switch x := v.(type) {
	case uint64:
		return int(x)
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(x)
		return n
	}
	return 0
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// appleScriptString quotes s for use inside an AppleScript string literal.
func appleScriptString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func osascript(script string) string { return "osascript -e " + shellQuote(script) }

// App is an application bundle.
type App struct {
	Name   string
	Bundle string // CFBundleIdentifier
	Path   string
	Agent  bool // LSUIElement: no Dock icon, usually lives in the menu bar
}

// ReadApp reads an application bundle's Info.plist.
func ReadApp(path string) (App, bool) {
	info := readPlist(filepath.Join(path, "Contents", "Info.plist"))
	if info == nil {
		return App{}, false
	}
	a := App{Path: path, Bundle: asString(info["CFBundleIdentifier"]), Agent: asBool(info["LSUIElement"]) && !asBool(info["LSBackgroundOnly"])}
	for _, k := range []string{"CFBundleDisplayName", "CFBundleName"} {
		if a.Name = asString(info[k]); a.Name != "" {
			break
		}
	}
	if a.Name == "" {
		a.Name = strings.TrimSuffix(filepath.Base(path), ".app")
	}
	return a, true
}

// InstalledApps lists the bundles in /Applications and ~/Applications (one
// level of subfolders included), keyed by bundle id.
func InstalledApps() map[string]App {
	apps := map[string]App{}
	for _, dir := range []string{"/Applications", filepath.Join(Home, "Applications"), "/System/Applications"} {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.app"))
		sub, _ := filepath.Glob(filepath.Join(dir, "*", "*.app"))
		for _, p := range append(matches, sub...) {
			if a, ok := ReadApp(p); ok && a.Bundle != "" {
				if _, seen := apps[a.Bundle]; !seen {
					apps[a.Bundle] = a
				}
			}
		}
	}
	return apps
}

// openSettings is the command that opens a System Settings pane.
func openSettings(pane string) string { return "open " + shellQuote("x-apple.systempreferences:"+pane) }

// Settings panes the changes point to when something must be done by hand.
const (
	PaneLoginItems = "com.apple.LoginItems-Settings.extension"
	PaneMenuBar    = "com.apple.ControlCenter-Settings.extension"
	PaneKeyboard   = "com.apple.Keyboard-Settings.extension"
)

// OpenSettings returns the command that opens a pane, for the CLI.
func OpenSettings(pane string) string { return openSettings(pane) }

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
