package macos

import (
	"sort"
	"strings"

	"github.com/lcajigasm/omega/internal/i18n"
)

// MenuBarApp is a running app that most likely shows an icon in the menu bar.
type MenuBarApp struct {
	App
	AtLogin  bool // it also opens at login or has a login helper
	Verdict  string
	Category string
	Why      string
}

// statusItemApps are regular apps (with a Dock icon) known to add a menu bar
// icon too. Apps without a Dock icon (LSUIElement) are detected on their own.
var statusItemApps = []string{"com.docker.docker", "com.getdropbox.dropbox", "com.tailscale.ipn.macos", "io.tailscale.ipn.macsys",
	"com.spotify.client", "com.1password.1password", "com.logi.optionsplus", "net.mullvad.vpn", "com.openai.chat", "com.anthropic.claudefordesktop"}

// menuBarManagers hide or rearrange menu bar icons; when one is installed,
// omega points the user to it instead of quitting apps.
var menuBarManagers = map[string]string{
	"com.surteesstudios.Bartender": "Bartender", "com.surteesstudios.Bartender-setapp": "Bartender",
	"com.jordanbaird.Ice": "Ice", "com.stonerl.Thaw": "Thaw", "com.dwarvesv.minimalbar": "Hidden Bar",
}

// MenuBarManager returns the name of an installed menu bar manager, if any,
// preferring one that is running.
func MenuBarManager(apps map[string]App) string {
	var found []App
	for _, a := range apps {
		if _, ok := menuBarManagers[a.Bundle]; ok || strings.EqualFold(a.Name, "Thaw") || strings.HasPrefix(a.Name, "Bartender") {
			found = append(found, a)
		}
	}
	if len(found) == 0 {
		return ""
	}
	running := map[string]bool{}
	for _, p := range runningBundles() {
		running[p] = true
	}
	sort.Slice(found, func(i, j int) bool {
		if running[found[i].Path] != running[found[j].Path] {
			return running[found[i].Path]
		}
		return found[i].Name < found[j].Name
	})
	return found[0].Name
}

// runningBundles returns the outermost and innermost app bundle of every
// running process that lives in an .app, outside /System.
func runningBundles() []string {
	out, err := Output("ps", "-axo", "comm=")
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var paths []string
	for _, l := range strings.Split(string(out), "\n") {
		i := strings.LastIndex(l, ".app/Contents/MacOS/")
		if i < 0 || strings.HasPrefix(l, "/System/") || strings.Contains(l[:i], "/Contents/Frameworks/") {
			continue
		}
		p := l[:i+4]
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths
}

// MenuBar lists the running apps that most likely have a menu bar icon.
func MenuBar(p *UserPolicy, items []Item) []MenuBarApp {
	atLogin := map[string]bool{}
	for _, it := range items {
		if it.Enabled && it.Bundle != "" && (it.Kind == KindLogin || it.Kind == KindHelper) {
			atLogin[it.Bundle] = true
		}
	}
	known := map[string]bool{}
	for _, id := range statusItemApps {
		known[id] = true
	}
	var apps []MenuBarApp
	seen := map[string]bool{}
	for _, path := range runningBundles() {
		a, ok := ReadApp(path)
		if !ok || a.Bundle == "" || seen[a.Bundle] || strings.HasPrefix(a.Bundle, "com.apple.") {
			continue
		}
		helper := strings.Contains(path, "/Contents/Library/LoginItems/") || strings.Contains(path, "/Contents/Helpers/") ||
			strings.HasSuffix(strings.ToLower(a.Name), "helper")
		if !(a.Agent && !helper) && !known[a.Bundle] {
			continue
		}
		seen[a.Bundle] = true
		m := MenuBarApp{App: a, AtLogin: atLogin[a.Bundle]}
		m.Verdict, m.Category, m.Why = p.MenuBarVerdict(a.Name, a.Bundle)
		apps = append(apps, m)
	}
	sort.Slice(apps, func(i, j int) bool { return strings.ToLower(apps[i].Name) < strings.ToLower(apps[j].Name) })
	return apps
}

// QuitChange quits a menu bar app now; it comes back at the next login if it
// still opens at login.
func (m MenuBarApp) QuitChange() Change {
	return Change{ID: "quit:" + m.Bundle, Section: "menubar", Title: i18n.T("mac.act.quit", m.Name),
		Command: osascript("quit app id " + appleScriptString(m.Bundle)), Undo: "open -gb " + shellQuote(m.Bundle)}
}

// HideChange opens the settings where a menu bar icon can be hidden: macOS 26
// and later list every app under Menu Bar → Allow in the Menu Bar.
func (m MenuBarApp) HideChange(manager string) Change {
	c := Change{ID: "hide:" + m.Bundle, Section: "menubar", Title: i18n.T("mac.act.hide", m.Name), Command: openSettings(PaneMenuBar),
		Manual: i18n.T("mac.manual.menubar", m.Name)}
	if manager != "" {
		c.Command = "open -a " + shellQuote(manager)
		c.Manual = i18n.T("mac.manual.menubar_manager", m.Name, manager)
	}
	return c
}
