package macos

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"howett.net/plist"

	"github.com/lcajigasm/omega/internal/i18n"
)

// DockApp is an app pinned to the Dock.
type DockApp struct {
	Label    string
	Bundle   string
	Path     string
	Missing  bool      // the app is no longer installed
	LastUsed time.Time // zero when unknown
	Index    int
}

// Dock is the Dock's contents and the settings omega can change.
type Dock struct {
	Apps        []DockApp
	Autohide    bool
	ShowRecents bool
	Magnify     bool
	TileSize    int
	Orientation string
}

// Dock settings omega knows how to change, with their type.
var DockSettings = map[string]string{
	"autohide": "bool", "show-recents": "bool", "magnification": "bool", "minimize-to-application": "bool",
	"tilesize": "int", "orientation": "string", "mineffect": "string", "autohide-delay": "float",
}

// Unused is how long without opening an app before omega suggests unpinning it.
const Unused = 90 * 24 * time.Hour

func tileApp(tile any) (DockApp, bool) {
	t, ok := tile.(map[string]any)
	if !ok {
		return DockApp{}, false
	}
	data, _ := t["tile-data"].(map[string]any)
	if data == nil {
		return DockApp{}, false
	}
	a := DockApp{Label: asString(data["file-label"]), Bundle: asString(data["bundle-identifier"])}
	if fd, ok := data["file-data"].(map[string]any); ok {
		a.Path = fileURLPath(asString(fd["_CFURLString"]))
	}
	return a, a.Label != "" || a.Bundle != ""
}

func lastUsed(path string) time.Time {
	out, err := Output("mdls", "-raw", "-name", "kMDItemLastUsedDate", path)
	if err != nil {
		return time.Time{}
	}
	t, _ := time.Parse("2006-01-02 15:04:05 -0700", strings.TrimSpace(string(out)))
	return t
}

// ReadDock reads the Dock. withUsage also asks Spotlight when each app was last opened.
func ReadDock(withUsage bool) (*Dock, error) {
	m := exportDomain("com.apple.dock")
	if m == nil {
		return nil, fmt.Errorf("could not read the Dock preferences")
	}
	d := &Dock{Autohide: asBool(m["autohide"]), ShowRecents: asBool(m["show-recents"]), Magnify: asBool(m["magnification"]),
		TileSize: asInt(m["tilesize"]), Orientation: asString(m["orientation"])}
	if _, ok := m["show-recents"]; !ok {
		d.ShowRecents = true // macOS default
	}
	if d.Orientation == "" {
		d.Orientation = "bottom"
	}
	tiles, _ := m["persistent-apps"].([]any)
	for i, t := range tiles {
		a, ok := tileApp(t)
		if !ok {
			continue
		}
		a.Index = i
		if a.Path != "" && strings.HasPrefix(a.Path, "/") {
			if _, err := os.Stat(a.Path); err != nil {
				a.Missing = true
			} else if withUsage {
				a.LastUsed = lastUsed(a.Path)
			}
		}
		d.Apps = append(d.Apps, a)
	}
	return d, nil
}

// Unused reports whether the app has not been opened for a long time.
func (a DockApp) Unused(now time.Time) bool {
	return !a.LastUsed.IsZero() && now.Sub(a.LastUsed) > Unused
}

// Matches reports whether the Dock app is the one named by a label, bundle id or path.
func (a DockApp) Matches(s string) bool {
	return strings.EqualFold(a.Label, s) || strings.EqualFold(a.Bundle, s) || (a.Path != "" && a.Path == strings.TrimSuffix(s, "/"))
}

// omegaBin is the command that runs omega's own Dock operations.
func omegaBin() string {
	exe, err := os.Executable()
	if err != nil {
		return "omega"
	}
	return shellQuote(exe)
}

// RemoveChange unpins an app from the Dock.
func (a DockApp) RemoveChange() Change {
	key := a.Bundle
	if key == "" {
		key = a.Label
	}
	c := Change{ID: "dock-remove:" + key, Section: "dock", Title: i18n.T("mac.act.dock_remove", a.Label),
		Command: fmt.Sprintf("%s mac op dock-remove %s", omegaBin(), shellQuote(key))}
	if a.Path != "" {
		c.Undo = fmt.Sprintf("%s mac op dock-add %s %d", omegaBin(), shellQuote(a.Path), a.Index)
	}
	return c
}

// AddChange pins an app to the end of the Dock.
func AddChange(app App) Change {
	return Change{ID: "dock-add:" + app.Path, Section: "dock", Title: i18n.T("mac.act.dock_add", app.Name),
		Command: fmt.Sprintf("%s mac op dock-add %s", omegaBin(), shellQuote(app.Path)),
		Undo:    fmt.Sprintf("%s mac op dock-remove %s", omegaBin(), shellQuote(app.Path))}
}

// SettingChange changes one Dock setting.
func (d *Dock) SettingChange(key, value string) (Change, error) {
	typ, ok := DockSettings[key]
	if !ok {
		return Change{}, fmt.Errorf("unknown Dock setting %q", key)
	}
	if typ == "bool" {
		v, err := strconv.ParseBool(value)
		if err != nil {
			return Change{}, fmt.Errorf("%s needs true or false", key)
		}
		value = strconv.FormatBool(v)
	}
	if (typ == "int" || typ == "float") && strings.Trim(value, "0123456789.") != "" {
		return Change{}, fmt.Errorf("%s needs a number", key)
	}
	write := func(v string) string {
		return fmt.Sprintf("defaults write com.apple.dock %s -%s %s && killall Dock", key, typ, shellQuote(v))
	}
	c := Change{ID: "dock-set:" + key + "=" + value, Section: "dock", Title: i18n.T("mac.act.dock_set", key, value), Command: write(value)}
	if old, err := Output("defaults", "read", "com.apple.dock", key); err == nil {
		o := strings.TrimSpace(string(old))
		if typ == "bool" {
			o = strconv.FormatBool(o == "1")
		}
		c.Undo = write(o)
	} else {
		c.Undo = fmt.Sprintf("defaults delete com.apple.dock %s && killall Dock", key)
	}
	return c, nil
}

// EditDock applies an edit to the Dock's persistent apps, writes the domain
// back and restarts the Dock. It is what `omega mac op` runs.
func EditDock(edit func(tiles []any) ([]any, error)) error {
	out, err := exec.Command("defaults", "export", "com.apple.dock", "-").Output()
	if err != nil {
		return fmt.Errorf("defaults export com.apple.dock: %w", err)
	}
	var m map[string]any
	format, err := plist.Unmarshal(out, &m)
	if err != nil {
		return err
	}
	tiles, _ := m["persistent-apps"].([]any)
	if tiles, err = edit(tiles); err != nil {
		return err
	}
	m["persistent-apps"] = tiles
	var buf bytes.Buffer
	enc := plist.NewEncoderForFormat(&buf, format)
	if err := enc.Encode(m); err != nil {
		return err
	}
	f, err := os.CreateTemp("", "omega-dock-*.plist")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(buf.Bytes()); err != nil {
		return err
	}
	f.Close()
	if out, err := exec.Command("defaults", "import", "com.apple.dock", f.Name()).CombinedOutput(); err != nil {
		return fmt.Errorf("defaults import: %s", strings.TrimSpace(string(out)))
	}
	return exec.Command("killall", "Dock").Run()
}

// DockRemove unpins the apps matching key (label, bundle id or path).
func DockRemove(key string) error {
	return EditDock(func(tiles []any) ([]any, error) { return removeTiles(tiles, key) })
}

func removeTiles(tiles []any, key string) ([]any, error) {
	var out []any
	for _, t := range tiles {
		if a, ok := tileApp(t); ok && a.Matches(key) {
			continue
		}
		out = append(out, t)
	}
	if len(out) == len(tiles) {
		return nil, fmt.Errorf("%q is not in the Dock", key)
	}
	return out, nil
}

// DockAdd pins an app at position index (-1 or past the end appends).
func DockAdd(path string, index int) error {
	path = strings.TrimSuffix(path, "/")
	app, ok := ReadApp(path)
	if !ok {
		return fmt.Errorf("%s is not an app", path)
	}
	return EditDock(func(tiles []any) ([]any, error) { return addTile(tiles, app, index) })
}

func addTile(tiles []any, app App, index int) ([]any, error) {
	for _, t := range tiles {
		if a, ok := tileApp(t); ok && (a.Path == app.Path || (app.Bundle != "" && a.Bundle == app.Bundle)) {
			return nil, fmt.Errorf("%s is already in the Dock", app.Name)
		}
	}
	tile := map[string]any{
		"tile-type": "file-tile",
		"tile-data": map[string]any{
			"file-label":        app.Name,
			"bundle-identifier": app.Bundle,
			"file-data":         map[string]any{"_CFURLString": (&url.URL{Scheme: "file", Path: app.Path + "/"}).String(), "_CFURLStringType": 15},
		},
	}
	if index < 0 || index >= len(tiles) {
		return append(tiles, tile), nil
	}
	return append(tiles[:index], append([]any{tile}, tiles[index:]...)...), nil
}
