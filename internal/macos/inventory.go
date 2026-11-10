package macos

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Sections of the inventory.
var Sections = []string{"login", "background", "menubar", "dock", "shortcuts", "permissions", "apps"}

// Inventory is everything omega mac looks at, read once.
type Inventory struct {
	Policy    *UserPolicy
	Apps      map[string]App
	Items     []Item
	MenuBar   []MenuBarApp
	Manager   string // installed menu bar manager
	Dock      *Dock
	Hotkeys   []Hotkey
	Conflicts []Conflict
	Now       time.Time
	// NoBTM is set when Background Task Management did not answer: login
	// items and app helpers are missing, only launchd jobs are listed.
	NoBTM bool

	Permissions []Permission
	// NoTCC is set when the user's privacy database could not be read: the
	// terminal needs Full Disk Access. System-wide entries are still listed.
	NoTCC     bool
	AppList   []AppInfo
	Leftovers []Leftover
}

// Load reads the sections asked for (all when none is given).
func Load(p *UserPolicy, sections ...string) (*Inventory, error) {
	want := map[string]bool{}
	for _, s := range sections {
		want[s] = true
	}
	all := len(want) == 0 || want["all"]
	inv := &Inventory{Policy: p, Now: time.Now()}
	// sfltool is by far the slowest read: start it first.
	needItems := all || want["login"] || want["background"] || want["menubar"]
	btm := make(chan string, 1)
	if needItems {
		go func() {
			out, ok := CachedBTM()
			if !ok {
				out = ""
			}
			btm <- out
		}()
	}
	inv.Apps = InstalledApps()
	// Sizing every app is slow too: do it while the rest is read.
	appsDone := make(chan struct{})
	go func() {
		defer close(appsDone)
		if all || want["apps"] {
			inv.AppList = Apps(p, inv.Apps, inv.Now)
			inv.Leftovers = Leftovers(p, inv.Apps)
		}
	}()
	defer func() { <-appsDone }()
	if all || want["permissions"] {
		inv.Permissions, inv.NoTCC = Permissions(p, inv.Apps)
	}
	if all || want["dock"] {
		d, err := ReadDock(true)
		if err != nil {
			return nil, err
		}
		inv.Dock = d
	}
	if all || want["shortcuts"] {
		inv.Hotkeys = Hotkeys(inv.Apps)
		inv.Conflicts = Conflicts(inv.Hotkeys, p)
	}
	if needItems {
		out := <-btm
		inv.NoBTM = out == ""
		inv.Items = Items(p, out)
	}
	if all || want["menubar"] {
		inv.MenuBar = MenuBar(p, inv.Items)
		inv.Manager = MenuBarManager(inv.Apps)
	}
	return inv, nil
}

// LoginItems are the enabled items that start at login.
func (inv *Inventory) LoginItems() []Item {
	var out []Item
	for _, it := range inv.Items {
		if it.Kind == KindLogin && it.Enabled {
			out = append(out, it)
		}
	}
	return out
}

// BackgroundItems are the jobs allowed to run in the background.
func (inv *Inventory) BackgroundItems() []Item {
	var out []Item
	for _, it := range inv.Items {
		if it.Background() {
			out = append(out, it)
		}
	}
	return out
}

// DockSuggestions are the Dock apps worth unpinning: missing, unused, or
// listed in the user's rules, plus the apps the rules want added.
func (inv *Inventory) DockSuggestions() (remove []DockApp, why map[string]string, add []App) {
	why = map[string]string{}
	if inv.Dock == nil {
		return
	}
	for _, a := range inv.Dock.Apps {
		switch {
		case a.Missing:
			why[a.Label] = "missing"
		case match(inv.Policy.Dock.Remove, a.Label, a.Bundle, a.Path):
			why[a.Label] = "rules"
		case a.Unused(inv.Now):
			why[a.Label] = "unused"
		default:
			continue
		}
		remove = append(remove, a)
	}
	for _, want := range inv.Policy.Dock.Add {
		in := false
		for _, a := range inv.Dock.Apps {
			if a.Matches(want) || match([]string{want}, a.Label) {
				in = true
			}
		}
		if in {
			continue
		}
		if app, ok := inv.FindApp(want); ok {
			add = append(add, app)
		}
	}
	return
}

// FindApp finds an installed app by name, bundle id or path.
func (inv *Inventory) FindApp(s string) (App, bool) {
	if strings.HasSuffix(strings.TrimSuffix(s, "/"), ".app") {
		return ReadApp(strings.TrimSuffix(s, "/"))
	}
	if a, ok := inv.Apps[s]; ok {
		return a, true
	}
	for _, a := range inv.Apps {
		if strings.EqualFold(a.Name, s) || strings.EqualFold(strings.TrimSuffix(filepath.Base(a.Path), ".app"), s) {
			return a, true
		}
	}
	return App{}, false
}

// matchItems finds the items a disable or enable id names, best match
// first. An app and its background-task record share the app's bundle id,
// and macOS keeps a switched-off record for every app that ever asked to
// open at login: the id means the item whose state the change would flip.
func (inv *Inventory) matchItems(op, arg string) []Item {
	var flips, rest []Item
	for _, it := range inv.Items {
		if it.ID() != arg && !strings.EqualFold(it.Name, arg) {
			continue
		}
		switch {
		case it.Enabled == (op == "disable"):
			flips = append(flips, it)
		case it.Kind != KindLogin:
			rest = append(rest, it)
		}
	}
	return append(flips, rest...)
}

// Resolve turns a change id from the report (e.g. "disable:com.google.keystone.agent",
// "hotkey-set:sys:64=ctrl+opt+space") into the change to run.
func (inv *Inventory) Resolve(id string) (Change, error) {
	op, arg, _ := strings.Cut(strings.TrimSpace(id), ":")
	if arg == "" {
		return Change{}, fmt.Errorf("change ids look like op:target, e.g. disable:com.google.keystone.agent")
	}
	switch op {
	case "disable", "enable":
		for _, it := range inv.matchItems(op, arg) {
			{
				if op == "enable" {
					c, ok := it.EnableChange()
					if !ok {
						return Change{}, fmt.Errorf("%s cannot be turned back on from here", it.Name)
					}
					return c, nil
				}
				return it.DisableChange(), nil
			}
		}
		return Change{}, fmt.Errorf("no login or background item %q", arg)
	case "quit", "hide":
		for _, m := range inv.MenuBar {
			if m.Bundle == arg || strings.EqualFold(m.Name, arg) {
				if op == "quit" {
					return m.QuitChange(), nil
				}
				return m.HideChange(inv.Manager), nil
			}
		}
		return Change{}, fmt.Errorf("no running menu bar app %q", arg)
	case "dock-remove":
		if inv.Dock != nil {
			for _, a := range inv.Dock.Apps {
				if a.Matches(arg) {
					return a.RemoveChange(), nil
				}
			}
		}
		return Change{}, fmt.Errorf("%q is not in the Dock", arg)
	case "dock-add":
		app, ok := inv.FindApp(arg)
		if !ok {
			return Change{}, fmt.Errorf("no installed app %q", arg)
		}
		return AddChange(app), nil
	case "dock-set":
		key, val, ok := strings.Cut(arg, "=")
		if !ok || inv.Dock == nil {
			return Change{}, fmt.Errorf("dock-set needs key=value, e.g. dock-set:autohide=true")
		}
		return inv.Dock.SettingChange(key, val)
	case "perm-reset":
		for _, x := range inv.Permissions {
			if x.ID() == arg {
				return x.ResetChange(), nil
			}
		}
		return Change{}, fmt.Errorf("no privacy entry %q; ids look like perm-reset:SystemPolicyAllFiles:/path/to/node", arg)
	case "perm-reset-all":
		for _, x := range inv.Permissions {
			if ShortService(x.Service) == ShortService(arg) {
				return ResetServiceChange(x.Service), nil
			}
		}
		return Change{}, fmt.Errorf("no privacy entries for %q", arg)
	case "app-remove":
		for _, a := range inv.AppList {
			if a.Bundle == arg || strings.EqualFold(a.Name, arg) {
				return a.UninstallChange(time.Now()), nil
			}
		}
		return Change{}, fmt.Errorf("no installed app %q", arg)
	case "leftover-trash":
		for _, l := range inv.Leftovers {
			if strings.EqualFold(l.Bundle, arg) {
				return l.TrashChange(time.Now()), nil
			}
		}
		return Change{}, fmt.Errorf("no leftovers for %q", arg)
	case "hotkey-off":
		h, ok := FindHotkey(inv.Hotkeys, arg)
		if !ok {
			return Change{}, fmt.Errorf("no shortcut %q", arg)
		}
		return h.DisableChange(), nil
	case "hotkey-set":
		i := strings.LastIndex(arg, "=")
		if i < 0 {
			return Change{}, fmt.Errorf("hotkey-set needs id=combo, e.g. hotkey-set:sys:64=ctrl+opt+space")
		}
		h, ok := FindHotkey(inv.Hotkeys, arg[:i])
		if !ok {
			return Change{}, fmt.Errorf("no shortcut %q", arg[:i])
		}
		c, err := ParseCombo(arg[i+1:])
		if err != nil {
			return Change{}, err
		}
		for _, o := range inv.Hotkeys {
			if o.Enabled && o.Combo == c && o.ID() != h.ID() {
				return Change{}, fmt.Errorf("%s is already used by %s (%s)", c, o.Owner, o.Action)
			}
		}
		return h.SetChange(c), nil
	}
	return Change{}, fmt.Errorf("unknown change %q", op)
}

// Report is the inventory as text for the agent: every item with omega's
// verdict and the change ids it can pass to mac_change.
func (inv *Inventory) Report(section string) string {
	var b strings.Builder
	all := section == "" || section == "all"
	if inv.NoBTM && (all || section == "login" || section == "background") {
		b.WriteString("NOTE: macOS's Background Task Management did not answer (sfltool dumpbtm timed out), so login items and app helpers are missing below; only launchd jobs are listed. Try again later.\n\n")
	}
	if all || section == "login" {
		b.WriteString("## Open at login\n")
		for _, it := range inv.LoginItems() {
			fmt.Fprintf(&b, "- %s [%s] verdict=%s category=%s%s — change: disable:%s\n", it.Name, orDash(it.Bundle), it.Verdict, it.Category, why(it.Why), it.ID())
		}
		b.WriteString("\n")
	}
	if all || section == "background" {
		b.WriteString("## Allowed in the background (launchd jobs, login helpers, background tasks)\n")
		for _, it := range inv.BackgroundItems() {
			state := "on"
			if !it.Enabled {
				state = "off"
			}
			var flags []string
			for f, on := range map[string]bool{"runs-at-load": it.RunAtLoad, "keep-alive": it.KeepAlive, "running": it.Running, "program-missing": it.Missing, "needs-sudo": it.System} {
				if on {
					flags = append(flags, f)
				}
			}
			sort.Strings(flags)
			fmt.Fprintf(&b, "- %s (%s, %s) %s [%s] %s verdict=%s category=%s%s", it.Name, it.Kind, state, orDash(it.Developer), orDash(it.Label),
				strings.Join(flags, ","), it.Verdict, it.Category, why(it.Why))
			if it.Enabled && it.ManualOnly() {
				fmt.Fprintf(&b, " — MANUAL ONLY: change disable:%s just opens the Login Items pane", it.ID())
			} else if it.Enabled {
				fmt.Fprintf(&b, " — change: disable:%s", it.ID())
			} else if _, ok := it.EnableChange(); ok {
				fmt.Fprintf(&b, " — change: enable:%s", it.ID())
			}
			b.WriteString("\n")
		}
		b.WriteString("Items marked MANUAL ONLY are switches macOS gives to the user alone (Allow in the Background): no tool can flip them. " +
			"Do not offer them one by one and do not call mac_change for each: list them together as something to finish by hand, " +
			"and open the pane once.\n\n")
	}
	if all || section == "menubar" {
		b.WriteString("## Menu bar apps (running apps that most likely show a menu bar icon)\n")
		if inv.Manager != "" {
			fmt.Fprintf(&b, "Menu bar manager installed: %s (hiding icons is best done there).\n", inv.Manager)
		}
		for _, m := range inv.MenuBar {
			fmt.Fprintf(&b, "- %s [%s] opens-at-login=%v verdict=%s category=%s%s — changes: quit:%s hide:%s\n",
				m.Name, m.Bundle, m.AtLogin, m.Verdict, m.Category, why(m.Why), m.Bundle, m.Bundle)
		}
		b.WriteString("\n")
	}
	if all || section == "dock" {
		if d := inv.Dock; d != nil {
			fmt.Fprintf(&b, "## Dock\nsettings: autohide=%v show-recents=%v magnification=%v tilesize=%d orientation=%s — change: dock-set:<key>=<value> (keys: autohide, show-recents, magnification, minimize-to-application, tilesize, orientation, mineffect, autohide-delay)\n",
				d.Autohide, d.ShowRecents, d.Magnify, d.TileSize, d.Orientation)
			for _, a := range d.Apps {
				used := "unknown"
				if !a.LastUsed.IsZero() {
					used = fmt.Sprintf("%d days ago", int(inv.Now.Sub(a.LastUsed).Hours()/24))
				}
				note := ""
				if a.Missing {
					note = " NOT INSTALLED"
				}
				fmt.Fprintf(&b, "- %d. %s [%s] last opened %s%s — change: dock-remove:%s\n", a.Index+1, a.Label, orDash(a.Bundle), used, note, orDash(a.Bundle))
			}
			_, _, add := inv.DockSuggestions()
			for _, a := range add {
				fmt.Fprintf(&b, "- the user's rules want %s in the Dock — change: dock-add:%s\n", a.Name, a.Path)
			}
			b.WriteString("Add any installed app with dock-add:<app name or path>.\n\n")
		}
	}
	if all || section == "shortcuts" {
		b.WriteString("## Global keyboard shortcuts\n")
		if len(inv.Conflicts) == 0 {
			b.WriteString("No conflicts found.\n")
		}
		used := Used(inv.Hotkeys)
		for _, c := range inv.Conflicts {
			fmt.Fprintf(&b, "CONFLICT %s (%s):\n", c.Combo, c.Combo.Config())
			for i, h := range c.Hotkeys {
				role := "loses"
				if i == c.Winner {
					role = "keeps it"
				}
				fmt.Fprintf(&b, "  - %s: %s — %s [id %s]%s\n", h.Owner, h.Action, role, h.ID(), assumed(h))
				if i != c.Winner {
					var alts []string
					for _, a := range Alternatives(h, used, 3) {
						alts = append(alts, a.Config())
					}
					fmt.Fprintf(&b, "    free alternatives: %s — changes: hotkey-set:%s=<combo>, hotkey-off:%s; set by hand at: %s\n",
						strings.Join(alts, ", "), h.ID(), h.ID(), h.Where)
				}
			}
			fmt.Fprintf(&b, "  recommendation: %s\n", c.Reason)
		}
		b.WriteString("\nAll enabled shortcuts:\n")
		for _, h := range inv.Hotkeys {
			if h.Enabled {
				fmt.Fprintf(&b, "- %s %s: %s [id %s]%s\n", h.Combo, h.Owner, h.Action, h.ID(), assumed(h))
			}
		}
		var unknown []string
		for _, m := range inv.MenuBar {
			found := false
			for _, h := range inv.Hotkeys {
				if h.Bundle == m.Bundle {
					found = true
				}
			}
			if !found {
				unknown = append(unknown, m.Name)
			}
		}
		if len(unknown) > 0 {
			fmt.Fprintf(&b, "\nRunning menu bar apps whose shortcuts omega does not know (they may register global shortcuts): %s\n", strings.Join(unknown, ", "))
		}
	}
	if all || section == "permissions" {
		inv.reportPermissions(&b)
	}
	if all || section == "apps" {
		inv.reportApps(&b)
	}
	return b.String()
}

func (inv *Inventory) reportPermissions(b *strings.Builder) {
	b.WriteString("\n## Privacy permissions (System Settings → Privacy & Security)\n")
	if inv.NoTCC {
		b.WriteString("NOTE: the user's privacy database could not be read, so only system-wide permissions (Full Disk Access, Accessibility, " +
			"Screen Recording, Input Monitoring, Developer Tools, App Management) are listed. To see the rest (Files & Folders, Automation, " +
			"Camera, Microphone, data from other apps), the terminal that runs omega needs Full Disk Access: tell the user, and offer to open " +
			"the pane with `" + PrivacyPane("SystemPolicyAllFiles") + "`.\n")
	}
	stale := inv.StalePermissions()
	fmt.Fprintf(b, "%d entries, %d left over.\n", len(inv.Permissions), len(stale))
	for _, x := range stale {
		what := "binary whose path no longer exists"
		if x.Why == WhyNotInstalled {
			what = "no installed app has this bundle id (Spotlight found none either)"
		}
		copies := ""
		if x.Copies > 1 {
			copies = fmt.Sprintf(", one of %d entries named %s", x.Copies, filepath.Base(x.Client))
		}
		how := "change"
		if x.ManualOnly() {
			how = "MANUAL ONLY (system-wide list, only System Settings can edit it): the change opens the pane and names the entry"
		}
		fmt.Fprintf(b, "- %s — %s [%s] verdict=%s (%s%s) — %s: perm-reset:%s\n", ServiceName(x.Service), x.Name, x.Client, x.Verdict, what, copies, how, x.ID())
	}
	// Binaries that exist, grouped: several live copies under one name are
	// worth explaining even though none is removed.
	type key struct{ service, base string }
	live := map[key][]Permission{}
	var order []key
	for _, x := range inv.Permissions {
		if x.IsPath && !x.Missing {
			k := key{x.Service, filepath.Base(x.Client)}
			if len(live[k]) == 0 {
				order = append(order, k)
			}
			live[k] = append(live[k], x)
		}
	}
	for _, k := range order {
		if len(live[k]) < 2 {
			continue
		}
		fmt.Fprintf(b, "- %s lists %d different %q binaries that all still exist:", ServiceName(k.service), len(live[k]), k.base)
		for _, x := range live[k] {
			fmt.Fprintf(b, " %s;", x.Client)
		}
		b.WriteString(" (each change: perm-reset:<Service>:<path>)\n")
	}
	b.WriteString("Entries marked MANUAL ONLY cannot be removed by any tool: offer them together as one thing to finish by hand, not as changes that will be applied.\n")
	b.WriteString("Removing an entry cannot be undone, but it is harmless: the app or binary asks again when it needs the permission. " +
		"perm-reset-all:<Service> forgets a permission for every app; offer it only when single entries could not be removed.\n")
}

func (inv *Inventory) reportApps(b *strings.Builder) {
	b.WriteString("\n## Installed apps (Apple's own left out), largest first\n")
	for i, a := range inv.AppList {
		if a.Verdict == Keep && i >= 15 {
			continue
		}
		used := "last opened unknown"
		if !a.LastUsed.IsZero() {
			used = fmt.Sprintf("last opened %d days ago", int(inv.Now.Sub(a.LastUsed).Hours()/24))
		}
		var notes []string
		if s := a.Source(); s != "" {
			notes = append(notes, "from "+s)
		}
		if a.Intel {
			notes = append(notes, "Intel only: runs under Rosetta")
		}
		fmt.Fprintf(b, "- %s [%s] %s, %s%s verdict=%s%s — change: app-remove:%s\n", a.Name, a.Bundle, HumanSize(a.Size), used,
			why(strings.Join(notes, ", ")), a.Verdict, why(a.Why), a.Bundle)
	}
	b.WriteString("app-remove uninstalls through Homebrew when a cask installed the app, otherwise moves it to the Trash. Never preselect it: " +
		"the user decides which apps go. For Intel-only apps, check whether an Apple silicon build exists before suggesting anything.\n")
	b.WriteString("\n## Leftovers in ~/Library of apps that are no longer installed\n")
	if len(inv.Leftovers) == 0 {
		b.WriteString("None above 5 MB.\n")
	}
	for _, l := range inv.Leftovers {
		note := ""
		if l.Vendor {
			note = " (another app of the same vendor is installed and may use it)"
		}
		fmt.Fprintf(b, "- %s %s in %d places%s — change: leftover-trash:%s\n", l.Bundle, HumanSize(l.Size), len(l.Paths), note, l.Bundle)
	}
	b.WriteString("leftover-trash moves the files to the Trash (undo puts them back). Application Support can hold the user's own data: " +
		"say what each one is before offering it, and never preselect one you cannot identify.\n")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func why(s string) string {
	if s == "" {
		return ""
	}
	return " (" + s + ")"
}

func assumed(h Hotkey) string {
	if h.Assumed {
		return " (the app's default; its settings could not be read, so the user may have changed it)"
	}
	return ""
}
