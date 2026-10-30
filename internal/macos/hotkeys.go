package macos

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/lcajigasm/omega/internal/i18n"
)

// SystemOwner is the owner name of macOS's own shortcuts.
const SystemOwner = "macOS"

// Hotkey is a global keyboard shortcut and who owns it.
type Hotkey struct {
	Owner   string // "macOS" or the app's name
	Bundle  string // app bundle id; empty for macOS
	Action  string
	Combo   Combo
	Enabled bool
	// Assumed is true when omega knows the app's default shortcut but could
	// not read the app's settings to confirm it.
	Assumed bool
	Where   string // where to change it by hand

	sysID     int    // com.apple.symbolichotkeys id
	domain    string // preferences domain or plist path (without .plist) of the app setting
	prefKey   string
	format    string // "ks" (KeyboardShortcuts JSON) or "mas" (keyCode/modifierFlags dict)
	raw       any    // the stored value, for undo
	sysRaw    map[string]any
	essential bool
}

// ID is the stable id used on cards and by the agent.
func (h Hotkey) ID() string {
	switch {
	case h.sysID != 0:
		return fmt.Sprintf("sys:%d", h.sysID)
	case h.prefKey != "":
		return "app:" + h.Bundle + ":" + h.prefKey
	}
	return "app:" + h.Bundle + ":" + slug(h.Action)
}

// slug turns "Magnet: left half" into "magnet-left-half".
func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// Changeable reports whether omega can change the shortcut itself.
func (h Hotkey) Changeable() bool { return h.sysID != 0 || h.format != "" }

type sysHotkey struct {
	id        int
	action    string
	combo     string
	on        bool // enabled by default
	essential bool
}

// systemHotkeys are macOS's global shortcuts in com.apple.symbolichotkeys,
// with their default combos.
var systemHotkeys = []sysHotkey{
	{28, "Save picture of screen as a file", "cmd+shift+3", true, true},
	{29, "Copy picture of screen to the clipboard", "ctrl+cmd+shift+3", true, true},
	{30, "Save picture of selected area as a file", "cmd+shift+4", true, true},
	{31, "Copy picture of selected area to the clipboard", "ctrl+cmd+shift+4", true, true},
	{184, "Screenshot and recording options", "cmd+shift+5", true, true},
	{64, "Show Spotlight search", "cmd+space", true, false},
	{65, "Show Finder search window", "opt+cmd+space", true, false},
	{60, "Select the previous input source", "ctrl+space", true, false},
	{61, "Select next source in Input menu", "ctrl+opt+space", true, false},
	{32, "Mission Control", "ctrl+up", true, true},
	{33, "Application windows", "ctrl+down", true, false},
	{36, "Show Desktop", "f11", true, false},
	{79, "Move left a space", "ctrl+left", true, true},
	{81, "Move right a space", "ctrl+right", true, true},
	{118, "Switch to Desktop 1", "ctrl+1", false, false},
	{119, "Switch to Desktop 2", "ctrl+2", false, false},
	{120, "Switch to Desktop 3", "ctrl+3", false, false},
	{52, "Turn Dock hiding on/off", "opt+cmd+d", true, false},
	{27, "Move focus to next window", "cmd+`", true, true},
	{7, "Move focus to the menu bar", "ctrl+f2", true, false},
	{8, "Move focus to the Dock", "ctrl+f3", true, false},
	{9, "Move focus to active or next window", "ctrl+f4", true, false},
	{10, "Move focus to the window toolbar", "ctrl+f5", true, false},
	{11, "Move focus to the floating window", "ctrl+f6", true, false},
	{12, "Turn keyboard access on or off", "ctrl+f1", true, false},
	{13, "Change the way Tab moves focus", "ctrl+f7", true, false},
	{57, "Move focus to status menus", "ctrl+f8", true, false},
	{98, "Show Help menu", "cmd+shift+/", true, false},
	{59, "Turn VoiceOver on or off", "cmd+f5", true, false},
	{162, "Show Accessibility controls", "opt+cmd+f5", true, false},
	{15, "Turn zoom on or off", "opt+cmd+8", false, false},
	{17, "Zoom in", "opt+cmd+=", false, false},
	{19, "Zoom out", "opt+cmd+-", false, false},
	{21, "Invert colours", "ctrl+opt+cmd+8", false, false},
}

// appHotkey is the default global shortcut of a well-known app.
type appHotkey struct {
	bundles   []string
	action    string
	combo     string
	where     string // English; the agent translates it
	prefKey   string // the app's preference key, when it stores the shortcut there
	format    string
	essential bool
}

// knownAppHotkeys are the default global shortcuts of popular apps that do not
// write them to their preferences until the user changes them.
var knownAppHotkeys = []appHotkey{
	{bundles: []string{"com.openai.chat", "com.openai.codex"}, action: "Open the ChatGPT launcher", combo: "opt+space",
		where: "ChatGPT → Settings → General → Keyboard shortcut"},
	{bundles: []string{"com.anthropic.claudefordesktop"}, action: "Claude Quick Entry", combo: "opt+space",
		where: "Claude → Settings → General → Quick Entry shortcut"},
	{bundles: []string{"com.microsoft.copilot-mac", "com.microsoft.copilot"}, action: "Open Copilot", combo: "opt+space",
		where: "Copilot → Settings → Keyboard shortcut"},
	{bundles: []string{"com.runningwithcrayons.Alfred"}, action: "Show Alfred", combo: "opt+space", essential: true,
		where: "Alfred Preferences → General → Alfred Hotkey"},
	{bundles: []string{"com.raycast.macos"}, action: "Show Raycast", combo: "opt+space", essential: true,
		where: "Raycast → Settings → General → Raycast Hotkey"},
	{bundles: []string{"com.1password.1password"}, action: "1Password Quick Access", combo: "cmd+shift+space", essential: true,
		where: "1Password → Settings → General → Keyboard shortcuts"},
	{bundles: []string{"com.culturedcode.ThingsMac"}, action: "Things Quick Entry", combo: "ctrl+space",
		where: "Things → Settings → General → Quick Entry"},
	{bundles: []string{"com.culturedcode.ThingsMac"}, action: "Things Quick Entry with Autofill", combo: "ctrl+opt+space",
		where: "Things → Settings → General → Quick Entry with Autofill"},
	{bundles: []string{"com.superultra.Homerow"}, action: "Homerow", combo: "cmd+shift+space", where: "Homerow → Settings → Shortcut"},
	{bundles: []string{"com.lwouis.alt-tab-macos"}, action: "AltTab", combo: "opt+tab", essential: true, where: "AltTab → Settings → Controls"},
	{bundles: []string{"com.knollsoft.Rectangle"}, action: "Rectangle: left half", combo: "ctrl+opt+left", prefKey: "leftHalf", format: "mas", where: "Rectangle → Settings"},
	{bundles: []string{"com.knollsoft.Rectangle"}, action: "Rectangle: right half", combo: "ctrl+opt+right", prefKey: "rightHalf", format: "mas", where: "Rectangle → Settings"},
	{bundles: []string{"com.knollsoft.Rectangle"}, action: "Rectangle: maximize", combo: "ctrl+opt+return", prefKey: "maximize", format: "mas", where: "Rectangle → Settings"},
	{bundles: []string{"com.crowdcafe.windowmagnet"}, action: "Magnet: left half", combo: "ctrl+opt+left", where: "Magnet → Settings → Keyboard"},
	{bundles: []string{"com.crowdcafe.windowmagnet"}, action: "Magnet: right half", combo: "ctrl+opt+right", where: "Magnet → Settings → Keyboard"},
	{bundles: []string{"com.crowdcafe.windowmagnet"}, action: "Magnet: maximize", combo: "ctrl+opt+return", where: "Magnet → Settings → Keyboard"},
}

func mustCombo(s string) Combo {
	c, err := ParseCombo(s)
	if err != nil {
		panic(err)
	}
	return c
}

// systemParams reads the combo stored in a symbolichotkeys entry.
func systemParams(entry map[string]any) (Combo, bool) {
	val, _ := entry["value"].(map[string]any)
	params, _ := val["parameters"].([]any)
	if len(params) < 3 {
		return Combo{}, false
	}
	return Combo{Key: asInt(params[1]), Mods: fromNS(asInt(params[2]))}, true
}

func systemHotkeyList() []Hotkey {
	stored := map[string]any{}
	if m := exportDomain("com.apple.symbolichotkeys"); m != nil {
		stored, _ = m["AppleSymbolicHotKeys"].(map[string]any)
	}
	var out []Hotkey
	for _, s := range systemHotkeys {
		h := Hotkey{Owner: SystemOwner, Action: s.action, Combo: mustCombo(s.combo), Enabled: s.on, sysID: s.id, essential: s.essential,
			Where: "System Settings → Keyboard → Keyboard Shortcuts"}
		if e, ok := stored[strconv.Itoa(s.id)].(map[string]any); ok {
			h.sysRaw = e
			if _, ok := e["enabled"]; ok {
				h.Enabled = asBool(e["enabled"])
			}
			if c, ok := systemParams(e); ok {
				h.Combo = c
			}
		}
		out = append(out, h)
	}
	return out
}

// inputSources counts the enabled keyboard layouts and input methods: with
// only one, the input source shortcuts do nothing.
func inputSources() int {
	m := exportDomain("com.apple.HIToolbox")
	list, _ := m["AppleEnabledInputSources"].([]any)
	n := 0
	for _, x := range list {
		e, _ := x.(map[string]any)
		switch asString(e["InputSourceKind"]) {
		case "Keyboard Layout", "Input Mode":
			n++
		}
	}
	return n
}

// prefFiles returns an app's preference plists: the regular one and the
// sandbox container's.
func prefFiles(bundle string) []string {
	var out []string
	for _, p := range []string{
		filepath.Join(Home, "Library/Preferences", bundle+".plist"),
		filepath.Join(Home, "Library/Containers", bundle, "Data/Library/Preferences", bundle+".plist"),
	} {
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// prefHotkeys reads the shortcuts an app stored in its preferences with the
// KeyboardShortcuts library (JSON) or MASShortcut/ShortcutRecorder (a dict
// with keyCode and modifierFlags).
func prefHotkeys(app App) []Hotkey {
	var out []Hotkey
	for _, file := range prefFiles(app.Bundle) {
		m := readPlist(file)
		domain := app.Bundle
		if strings.Contains(file, "/Containers/") {
			domain = strings.TrimSuffix(file, ".plist")
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := m[k]
			h := Hotkey{Owner: app.Name, Bundle: app.Bundle, domain: domain, prefKey: k, raw: v, Enabled: true,
				Where: app.Name + " → Settings"}
			switch {
			case strings.HasPrefix(k, "KeyboardShortcuts_"):
				h.format = "ks"
				h.Action = humanize(strings.TrimPrefix(k, "KeyboardShortcuts_"))
				s, isString := v.(string)
				if !isString {
					continue // false: the user turned it off
				}
				var ks struct {
					Code int `json:"carbonKeyCode"`
					Mods int `json:"carbonModifiers"`
				}
				if json.Unmarshal([]byte(s), &ks) != nil {
					continue
				}
				h.Combo = Combo{Key: ks.Code, Mods: fromCarbon(ks.Mods)}
			default:
				d, ok := v.(map[string]any)
				if !ok {
					continue
				}
				code, hasCode := d["keyCode"]
				flags, hasFlags := d["modifierFlags"]
				if !hasCode || !hasFlags {
					continue
				}
				h.format = "mas"
				h.Action = humanize(k)
				h.Combo = Combo{Key: asInt(code), Mods: fromNS(asInt(flags))}
			}
			if h.Combo.Mods == 0 && !isFunctionKey(h.Combo.Key) {
				continue // a plain key cannot be a global shortcut
			}
			out = append(out, h)
		}
	}
	return out
}

func isFunctionKey(code int) bool {
	k, ok := byCode[code]
	return ok && strings.HasPrefix(k.name, "f") && len(k.name) > 1
}

// humanize turns "toggleLauncher" or "show_window" into "toggle launcher".
func humanize(s string) string {
	var b strings.Builder
	for i, r := range s {
		switch {
		case r == '_' || r == '-':
			b.WriteRune(' ')
		case i > 0 && r >= 'A' && r <= 'Z':
			b.WriteRune(' ')
			b.WriteRune(r + 32)
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// Hotkeys lists the global shortcuts of macOS and the installed apps.
func Hotkeys(apps map[string]App) []Hotkey {
	out := systemHotkeyList()
	if inputSources() <= 1 {
		for i := range out {
			if out[i].sysID == 60 || out[i].sysID == 61 {
				out[i].Action += " (only one input source: it does nothing)"
			}
		}
	}
	ids := make([]string, 0, len(apps))
	for id := range apps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	fromPrefs := map[string]map[string]bool{} // bundle → pref keys found
	for _, id := range ids {
		if strings.HasPrefix(id, "com.apple.") {
			continue
		}
		for _, h := range prefHotkeys(apps[id]) {
			if fromPrefs[id] == nil {
				fromPrefs[id] = map[string]bool{}
			}
			fromPrefs[id][h.prefKey] = true
			for _, k := range knownAppHotkeys {
				if k.prefKey == h.prefKey && containsStr(k.bundles, id) {
					h.Action, h.Where, h.essential = k.action, k.where, k.essential
				}
			}
			out = append(out, h)
		}
	}
	for _, k := range knownAppHotkeys {
		for _, b := range k.bundles {
			app, ok := apps[b]
			if !ok || (k.prefKey != "" && fromPrefs[b][k.prefKey]) {
				continue
			}
			out = append(out, Hotkey{Owner: app.Name, Bundle: b, Action: k.action, Combo: mustCombo(k.combo), Enabled: true,
				Assumed: true, Where: k.where, essential: k.essential, prefKey: k.prefKey, format: k.format, domain: b})
		}
	}
	return out
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Conflict is a shortcut wanted by more than one action.
type Conflict struct {
	Combo    Combo
	Hotkeys  []Hotkey
	Winner   int    // index into Hotkeys of the one that should keep it
	Reason   string // why the winner was chosen (English, for the agent)
	FromUser bool   // the winner comes from the user's rules
}

// Losers are the hotkeys that should give the combo up.
func (c Conflict) Losers() []Hotkey {
	var out []Hotkey
	for i, h := range c.Hotkeys {
		if i != c.Winner {
			out = append(out, h)
		}
	}
	return out
}

// rank orders hotkeys by how much they deserve a contested combo under
// omega's defaults: essential macOS features, then shortcuts the user set on
// purpose, then launchers and utilities, then the rest of macOS, then apps
// that claim a shortcut on install (AI assistants, chat).
func rank(h Hotkey) int {
	cat, _ := Categorize(h.Owner, h.Bundle)
	switch {
	case h.Owner == SystemOwner && h.essential:
		return 100
	case h.Owner == SystemOwner && strings.Contains(h.Action, "it does nothing"):
		return 20
	case h.Owner != SystemOwner && !h.Assumed:
		if cat == CatOnDemand {
			return 75
		}
		return 90
	case h.Owner == SystemOwner:
		return 70
	case h.essential || cat == CatEssential:
		return 60
	case cat == CatOnDemand:
		return 30
	}
	return 50
}

// Conflicts finds combos claimed by more than one enabled hotkey and picks
// who should keep each one.
func Conflicts(hks []Hotkey, p *UserPolicy) []Conflict {
	groups := map[string][]Hotkey{}
	for _, h := range hks {
		if h.Enabled {
			groups[h.Combo.Config()] = append(groups[h.Combo.Config()], h)
		}
	}
	var out []Conflict
	for _, key := range sortedKeys(groups) {
		g := groups[key]
		if len(g) < 2 {
			continue
		}
		c := Conflict{Combo: g[0].Combo, Hotkeys: g, Winner: -1}
		if owner, ok := p.Shortcuts.Owners[key]; ok {
			for i, h := range g {
				if strings.EqualFold(h.Owner, owner) || strings.EqualFold(h.Bundle, owner) {
					c.Winner, c.Reason, c.FromUser = i, "the user's rules give it to "+owner, true
					break
				}
			}
			if c.Winner < 0 && strings.EqualFold(owner, "none") {
				c.Reason, c.FromUser = "the user's rules give it to nobody", true
			}
		}
		if c.Winner < 0 && !c.FromUser {
			best := 0
			for i, h := range g {
				if rank(h) > rank(g[best]) {
					best = i
				}
			}
			c.Winner = best
			c.Reason = "omega's defaults: " + rankReason(g[best])
		}
		out = append(out, c)
	}
	return out
}

func rankReason(h Hotkey) string {
	switch r := rank(h); {
	case r >= 100:
		return "an essential macOS shortcut"
	case r >= 75:
		return "the user set it on purpose in " + h.Owner
	case r >= 70:
		return "a standard macOS shortcut"
	case r >= 60:
		return "a launcher or utility that depends on its shortcut"
	}
	return "the other owners matter less"
}

// Used returns every combo in use, for suggesting free alternatives.
func Used(hks []Hotkey) map[string]bool {
	m := map[string]bool{}
	for _, h := range hks {
		if h.Enabled {
			m[h.Combo.Config()] = true
		}
	}
	return m
}

// Alternatives suggests free combos for a hotkey that loses a conflict.
func Alternatives(h Hotkey, used map[string]bool, n int) []Combo {
	return alternatives(h.Combo, used, n)
}

// FindHotkey looks a hotkey up by its ID.
func FindHotkey(hks []Hotkey, id string) (Hotkey, bool) {
	for _, h := range hks {
		if h.ID() == id {
			return h, true
		}
	}
	return Hotkey{}, false
}

const activateSettings = "/System/Library/PrivateFrameworks/SystemAdministration.framework/Resources/activateSettings -u"

func xmlBool(b bool) string {
	if b {
		return "<true/>"
	}
	return "<false/>"
}

func sysEntryXML(enabled bool, c Combo) string {
	return fmt.Sprintf("<dict><key>enabled</key>%s<key>value</key><dict><key>parameters</key><array><integer>%d</integer>"+
		"<integer>%d</integer><integer>%d</integer></array><key>type</key><string>standard</string></dict></dict>",
		xmlBool(enabled), c.ascii(), c.Key, c.Mods.ns())
}

func (h Hotkey) sysWrite(enabled bool, c Combo) string {
	return fmt.Sprintf("defaults write com.apple.symbolichotkeys AppleSymbolicHotKeys -dict-add %d %s && %s",
		h.sysID, shellQuote(sysEntryXML(enabled, c)), activateSettings)
}

// appWrite quits the app, writes its preference and opens it again, so the
// running app does not overwrite the change.
func (h Hotkey) appWrite(value string) string {
	return fmt.Sprintf("%s 2>/dev/null; sleep 1; defaults write %s %s %s; open -gb %s",
		osascript("quit app id "+appleScriptString(h.Bundle)), shellQuote(h.domain), shellQuote(h.prefKey), value, shellQuote(h.Bundle))
}

func (h Hotkey) appValue(c Combo, enabled bool) string {
	switch h.format {
	case "ks":
		if !enabled {
			return "-bool false"
		}
		return "-string " + shellQuote(fmt.Sprintf(`{"carbonKeyCode":%d,"carbonModifiers":%d}`, c.Key, c.Mods.carbon()))
	case "mas":
		return fmt.Sprintf("-dict keyCode -int %d modifierFlags -int %d", c.Key, c.Mods.ns())
	}
	return ""
}

// undoApp restores the app preference as it was.
func (h Hotkey) undoApp() string {
	if h.Assumed {
		return fmt.Sprintf("%s 2>/dev/null; sleep 1; defaults delete %s %s; open -gb %s",
			osascript("quit app id "+appleScriptString(h.Bundle)), shellQuote(h.domain), shellQuote(h.prefKey), shellQuote(h.Bundle))
	}
	switch v := h.raw.(type) {
	case bool:
		return h.appWrite("-bool " + strconv.FormatBool(v))
	case string:
		return h.appWrite("-string " + shellQuote(v))
	case map[string]any:
		return h.appWrite(fmt.Sprintf("-dict keyCode -int %d modifierFlags -int %d", asInt(v["keyCode"]), asInt(v["modifierFlags"])))
	}
	return ""
}

func (h Hotkey) undoSys() string {
	if h.sysRaw == nil {
		def := Hotkey{sysID: h.sysID}
		for _, s := range systemHotkeys {
			if s.id == h.sysID {
				return def.sysWrite(s.on, mustCombo(s.combo))
			}
		}
		return ""
	}
	c, ok := systemParams(h.sysRaw)
	if !ok {
		c = h.Combo
	}
	return h.sysWrite(asBool(h.sysRaw["enabled"]), c)
}

// manual is the change for a shortcut omega cannot write: it opens the app
// and says where to change it.
func (h Hotkey) manual(title string) Change {
	cmd := openSettings(PaneKeyboard)
	if h.Bundle != "" {
		cmd = "open -b " + shellQuote(h.Bundle)
	}
	return Change{ID: "hotkey-manual:" + h.ID(), Section: "shortcuts", Title: title, Command: cmd, Manual: i18n.T("mac.manual.hotkey", h.Owner, h.Where)}
}

// DisableChange turns a shortcut off.
func (h Hotkey) DisableChange() Change {
	title := i18n.T("mac.act.hotkey_off", h.Combo.String(), h.Owner, h.Action)
	switch {
	case h.sysID != 0:
		return Change{ID: "hotkey-off:" + h.ID(), Section: "shortcuts", Title: title, Command: h.sysWrite(false, h.Combo), Undo: h.undoSys()}
	case h.format == "ks":
		return Change{ID: "hotkey-off:" + h.ID(), Section: "shortcuts", Title: title, Command: h.appWrite(h.appValue(h.Combo, false)), Undo: h.undoApp()}
	}
	return h.manual(title)
}

// SetChange moves a shortcut to another combo.
func (h Hotkey) SetChange(c Combo) Change {
	title := i18n.T("mac.act.hotkey_set", h.Owner, h.Action, h.Combo.String(), c.String())
	id := "hotkey-set:" + h.ID() + "=" + c.Config()
	switch {
	case h.sysID != 0:
		return Change{ID: id, Section: "shortcuts", Title: title, Command: h.sysWrite(true, c), Undo: h.undoSys()}
	case h.format != "":
		return Change{ID: id, Section: "shortcuts", Title: title, Command: h.appWrite(h.appValue(c, true)), Undo: h.undoApp()}
	}
	ch := h.manual(title)
	ch.Manual = i18n.T("mac.manual.hotkey_to", h.Owner, h.Where, c.String())
	return ch
}
