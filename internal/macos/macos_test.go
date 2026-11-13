package macos

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"howett.net/plist"
)

const btmSample = `========================
 Records for UID -2 : FFFFEEEE-DDDD-CCCC-BBBB-AAAAFFFFFFFE
========================

 Items:

 #1:
                 UUID: E237784D-D0DA-4F35-AFF4-828507E8C93E
                 Name: Mullvad VPN
       Developer Name: Mullvad VPN AB
                 Type: legacy daemon (0x10010)
          Disposition: [enabled, allowed, notified] (0xb)
           Identifier: 16.net.mullvad.daemon
                  URL: /Library/LaunchDaemons/net.mullvad.daemon.plist
      Executable Path: /Applications/Mullvad VPN.app/Contents/Resources/mullvad-daemon
    Assoc. Bundle IDs: [ net.mullvad.vpn ]

========================
 Records for UID 501 : 0845AA08-7EF1-4372-921E-8204A6C95D27
========================

 Items:

 #1:
                 Name: Alfred 5
       Developer Name: (null)
                 Type: app (0x2)
          Disposition: [enabled, allowed, notified] (0xb)
           Identifier: 2.com.runningwithcrayons.Alfred
                  URL: file:///Applications/Alfred%205.app/
    Bundle Identifier: com.runningwithcrayons.Alfred
  Embedded Item Identifiers:
    #1: 4.com.runningwithcrayons.Alfred-Helper

 #2:
                 Name: ChatGPT - background tasks
       Developer Name: OpenAI OpCo, LLC
                 Type: background tasks (0x2000)
          Disposition: [enabled, allowed, notified] (0xb)
           Identifier: 8192.com.openai.chat.bgtask
 #3:
                 Name: CodexDockTilePlugin.docktileplugin
                 Type: dock tile (0x80)
          Disposition: [enabled, allowed, notified] (0xb)
 #4:
                 Name: 1Password Launcher
       Developer Name: AgileBits Inc.
                 Type: login item (0x4)
          Disposition: [disabled, allowed, notified] (0xa)
           Identifier: 4.com.1password.1password-launcher

========================
 Records for UID 502 : 11111111-2222-3333-4444-555555555555
========================
 #1:
                 Name: Other user's app
                 Type: app (0x2)
          Disposition: [enabled, allowed, notified] (0xb)
`

func TestParseBTM(t *testing.T) {
	recs := parseBTM(btmSample, "501")
	if len(recs) != 5 {
		t.Fatalf("got %d records, want 5 (the daemon, four of uid 501, none of uid 502): %v", len(recs), recs)
	}
	if recs[0]["Name"] != "Mullvad VPN" || btmKind(recs[0]["Type"]) != KindDaemon || btmLabel(recs[0]["Identifier"]) != "net.mullvad.daemon" {
		t.Errorf("daemon: %v", recs[0])
	}
	if btmKind(recs[1]["Type"]) != KindLogin || fileURLPath(recs[1]["URL"]) != "/Applications/Alfred 5.app" {
		t.Errorf("login item: %v → %s", recs[1], fileURLPath(recs[1]["URL"]))
	}
	if btmKind(recs[2]["Type"]) != KindTask || btmKind(recs[3]["Type"]) != "" || btmKind(recs[4]["Type"]) != KindHelper {
		t.Errorf("kinds: %s %s %s", recs[2]["Type"], recs[3]["Type"], recs[4]["Type"])
	}
	if strings.Contains(recs[4]["Disposition"], "[enabled") {
		t.Errorf("the launcher is disabled: %v", recs[4])
	}
}

func TestCombos(t *testing.T) {
	for in, want := range map[string]string{
		"opt+space": "opt+space", "Option+Space": "opt+space", "⌃⌥Space": "ctrl+opt+space", "cmd+shift+4": "shift+cmd+4",
		"alt+cmd+left": "opt+cmd+left", "ctrl+f2": "ctrl+f2", "cmd+`": "cmd+`",
	} {
		c, err := ParseCombo(in)
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if c.Config() != want {
			t.Errorf("%s → %s, want %s", in, c.Config(), want)
		}
		back, _ := ParseCombo(c.Config())
		if back != c {
			t.Errorf("%s does not round-trip", in)
		}
	}
	if _, err := ParseCombo("hyper+q"); err == nil {
		t.Error("unknown modifier accepted")
	}
	c := mustCombo("ctrl+opt+space")
	if c.String() != "⌃⌥Space" || c.Mods.ns() != 786432 || fromNS(786432) != c.Mods || c.Mods.carbon() != 6144 || fromCarbon(6144) != c.Mods {
		t.Errorf("flags: %s ns=%d carbon=%d", c, c.Mods.ns(), c.Mods.carbon())
	}
}

func hk(owner, bundle, action, combo string, assumed bool) Hotkey {
	return Hotkey{Owner: owner, Bundle: bundle, Action: action, Combo: mustCombo(combo), Enabled: true, Assumed: assumed}
}

func TestConflictsDefaults(t *testing.T) {
	hks := []Hotkey{
		hk("ChatGPT", "com.openai.chat", "launcher", "opt+space", true),
		hk("Claude", "com.anthropic.claudefordesktop", "quick entry", "opt+space", true),
		hk("Alfred", "com.runningwithcrayons.Alfred", "show", "opt+space", true),
		hk("Things", "com.culturedcode.ThingsMac", "quick entry", "ctrl+space", true),
		{Owner: SystemOwner, Action: "Select the previous input source", Combo: mustCombo("ctrl+space"), Enabled: true, sysID: 60},
		{Owner: SystemOwner, Action: "Save picture of selected area", Combo: mustCombo("cmd+shift+4"), Enabled: true, sysID: 30, essential: true},
		hk("Shottr", "cc.ffitch.shottr", "area", "cmd+shift+4", false),
		hk("Raycast", "com.raycast.macos", "show", "cmd+k", true),
	}
	cs := Conflicts(hks, &UserPolicy{})
	if len(cs) != 3 {
		t.Fatalf("got %d conflicts, want 3", len(cs))
	}
	win := map[string]string{}
	for _, c := range cs {
		win[c.Combo.Config()] = c.Hotkeys[c.Winner].Owner
	}
	// Launchers beat AI assistants that claim the combo on install; macOS
	// beats an app default; an essential macOS shortcut beats everything.
	if win["opt+space"] != "Alfred" || win["ctrl+space"] != SystemOwner || win["shift+cmd+4"] != SystemOwner {
		t.Errorf("winners: %v", win)
	}

	p := &UserPolicy{}
	p.Shortcuts.Owners = map[string]string{}
	if err := p.AddRule("owner", "Option+Space=ChatGPT"); err != nil {
		t.Fatal(err)
	}
	for _, c := range Conflicts(hks, p) {
		if c.Combo.Config() == "opt+space" && (c.Hotkeys[c.Winner].Owner != "ChatGPT" || !c.FromUser) {
			t.Errorf("the user's rule should give opt+space to ChatGPT: %+v", c)
		}
	}
}

func TestAlternativesAreFree(t *testing.T) {
	h := hk("ChatGPT", "com.openai.chat", "launcher", "opt+space", true)
	used := map[string]bool{"opt+space": true, "opt+shift+space": true}
	alts := alternatives(h.Combo, used, 3)
	if len(alts) != 3 {
		t.Fatalf("alternatives: %v", alts)
	}
	for _, a := range alts {
		if used[a.Config()] || reserved[a.Config()] {
			t.Errorf("%s is taken", a.Config())
		}
	}
	if alts[0].Config() != "ctrl+opt+space" {
		t.Errorf("first alternative %s, want ctrl+opt+space", alts[0].Config())
	}
}

func TestPolicyVerdicts(t *testing.T) {
	p := &UserPolicy{}
	for _, tc := range []struct {
		names []string
		want  string
	}{
		{[]string{"GoogleUpdater", "", "com.google.GoogleUpdater.wake"}, Disable},
		{[]string{"Mullvad VPN", "net.mullvad.vpn", "net.mullvad.daemon"}, Keep},
		{[]string{"Claude", "com.anthropic.claudefordesktop"}, Disable},
		{[]string{"Mystery", "com.example.mystery"}, Review},
		{[]string{"Terminal", "com.apple.Terminal"}, Keep},
	} {
		if v, _, _ := p.StartupVerdict(tc.names...); v != tc.want {
			t.Errorf("%v: %s, want %s", tc.names, v, tc.want)
		}
	}
	p.Startup.Keep = []string{"claude"}
	p.Startup.Disable = []string{"com.example.*"}
	if v, _, _ := p.StartupVerdict("Claude", "com.anthropic.claudefordesktop"); v != Keep {
		t.Errorf("the user's keep rule should win: %s", v)
	}
	if v, _, _ := p.StartupVerdict("Mystery", "com.example.mystery"); v != Disable {
		t.Errorf("the user's disable rule should win: %s", v)
	}
}

func TestLoadPolicyNormalizesCombos(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	os.MkdirAll(filepath.Dir(PolicyPath()), 0o755)
	os.WriteFile(PolicyPath(), []byte("[shortcuts.owners]\n\"cmd+shift+space\" = \"1Password\"\n\"⌥Space\" = \"ChatGPT\"\n"), 0o644)
	p, err := LoadPolicy()
	if err != nil {
		t.Fatal(err)
	}
	if p.Shortcuts.Owners["shift+cmd+space"] != "1Password" || p.Shortcuts.Owners["opt+space"] != "ChatGPT" {
		t.Errorf("owners: %v", p.Shortcuts.Owners)
	}
}

func TestHotkeyChanges(t *testing.T) {
	sys := Hotkey{Owner: SystemOwner, Action: "Show Spotlight search", Combo: mustCombo("cmd+space"), Enabled: true, sysID: 64}
	set := sys.SetChange(mustCombo("ctrl+opt+space"))
	if !strings.Contains(set.Command, "-dict-add 64") || !strings.Contains(set.Command, "<integer>32</integer><integer>49</integer><integer>786432</integer>") ||
		!strings.Contains(set.Command, "activateSettings") {
		t.Errorf("set: %s", set.Command)
	}
	// Never changed before: undo restores the macOS default.
	if !strings.Contains(set.Undo, "<true/>") || !strings.Contains(set.Undo, "<integer>1048576</integer>") {
		t.Errorf("undo: %s", set.Undo)
	}
	if off := sys.DisableChange(); !strings.Contains(off.Command, "<false/>") {
		t.Errorf("off: %s", off.Command)
	}

	ks := Hotkey{Owner: "Shottr", Bundle: "cc.ffitch.shottr", Combo: mustCombo("cmd+shift+4"), Enabled: true,
		domain: "cc.ffitch.shottr", prefKey: "KeyboardShortcuts_area", format: "ks", raw: `{"carbonKeyCode":21,"carbonModifiers":768}`}
	c := ks.SetChange(mustCombo("ctrl+shift+4"))
	if !strings.Contains(c.Command, `{"carbonKeyCode":21,"carbonModifiers":4608}`) || !strings.Contains(c.Command, "quit app id") ||
		!strings.Contains(c.Undo, `"carbonModifiers":768`) {
		t.Errorf("KeyboardShortcuts change: %s / %s", c.Command, c.Undo)
	}

	assumed := hk("ChatGPT", "com.openai.chat", "Open the ChatGPT launcher", "opt+space", true)
	assumed.Where = "ChatGPT → Settings"
	m := assumed.SetChange(mustCombo("ctrl+opt+space"))
	if m.Undo != "" || m.Manual == "" || !strings.Contains(m.Command, "open -b 'com.openai.chat'") {
		t.Errorf("an app omega cannot write is a manual change: %+v", m)
	}
}

func writePlist(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := plist.Marshal(v, plist.BinaryFormat)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPrefHotkeys(t *testing.T) {
	old := Home
	Home = t.TempDir()
	defer func() { Home = old }()
	writePlist(t, filepath.Join(Home, "Library/Preferences/com.example.app.plist"), map[string]any{
		"KeyboardShortcuts_toggleLauncher": `{"carbonKeyCode":49,"carbonModifiers":2048}`,
		"KeyboardShortcuts_off":            false,
		"leftHalf":                         map[string]any{"keyCode": 123, "modifierFlags": 786432},
		"plainKey":                         map[string]any{"keyCode": 0, "modifierFlags": 0},
		"unrelated":                        "x",
	})
	hks := prefHotkeys(App{Name: "Example", Bundle: "com.example.app"})
	got := map[string]string{}
	for _, h := range hks {
		got[h.prefKey] = h.Combo.Config() + " " + h.format + " " + h.Action
	}
	want := map[string]string{
		"KeyboardShortcuts_toggleLauncher": "opt+space ks toggle launcher",
		"leftHalf":                         "ctrl+opt+left mas left half",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %q, want %q", k, got[k], v)
		}
	}
}

func dockTile(label, bundle, path string) any {
	return map[string]any{"tile-type": "file-tile", "tile-data": map[string]any{
		"file-label": label, "bundle-identifier": bundle,
		"file-data": map[string]any{"_CFURLString": "file://" + strings.ReplaceAll(path, " ", "%20") + "/", "_CFURLStringType": 15},
	}}
}

func TestDockEdits(t *testing.T) {
	tiles := []any{dockTile("Safari", "com.apple.Safari", "/Applications/Safari.app"), dockTile("Grok Bot", "com.anysphere.sand", "/Applications/Grok Bot.app")}
	if a, ok := tileApp(tiles[1]); !ok || a.Path != "/Applications/Grok Bot.app" {
		t.Fatalf("tile: %+v", a)
	}
	out, err := removeTiles(tiles, "com.anysphere.sand")
	if err != nil || len(out) != 1 {
		t.Fatalf("remove: %v %v", out, err)
	}
	if _, err := removeTiles(tiles, "Nope"); err == nil {
		t.Error("removing an app that is not in the Dock should fail")
	}
	out, err = addTile(out, App{Name: "Grok Bot", Bundle: "com.anysphere.sand", Path: "/Applications/Grok Bot.app"}, 0)
	if err != nil || len(out) != 2 {
		t.Fatalf("add: %v %v", out, err)
	}
	if a, _ := tileApp(out[0]); a.Bundle != "com.anysphere.sand" || a.Path != "/Applications/Grok Bot.app" {
		t.Errorf("added tile: %+v", a)
	}
	if _, err := addTile(out, App{Name: "Safari", Bundle: "com.apple.Safari", Path: "/Applications/Safari.app"}, -1); err == nil {
		t.Error("adding an app twice should fail")
	}
}

func TestResolveRefusesTakenCombo(t *testing.T) {
	inv := &Inventory{Hotkeys: []Hotkey{
		{Owner: SystemOwner, Action: "Spotlight", Combo: mustCombo("cmd+space"), Enabled: true, sysID: 64},
		{Owner: SystemOwner, Action: "Finder search", Combo: mustCombo("opt+cmd+space"), Enabled: true, sysID: 65},
	}}
	if _, err := inv.Resolve("hotkey-set:sys:64=opt+cmd+space"); err == nil {
		t.Error("moving a shortcut onto a taken combo should fail")
	}
	c, err := inv.Resolve("hotkey-set:sys:64=ctrl+opt+cmd+space")
	if err != nil || c.Section != "shortcuts" {
		t.Errorf("resolve: %+v %v", c, err)
	}
	if _, err := inv.Resolve("explode:everything"); err == nil {
		t.Error("unknown ops must fail")
	}
}

func TestJournal(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := Record(Change{ID: "a", Title: "first", Command: "true", Undo: "true"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	Record(Change{ID: "b", Title: "second", Command: "true", Undo: "true"})
	j := Journal()
	if len(j) != 2 || j[0].ID != "b" {
		t.Fatalf("newest first: %+v", j)
	}
	if err := MarkUndone(map[time.Time]bool{j[0].Time: true}); err != nil {
		t.Fatal(err)
	}
	j = Journal()
	if !j[0].Undone || j[1].Undone {
		t.Errorf("undone flags: %+v", j)
	}
}

// Undoing a move to the Trash fails when the place is taken again: it must
// not report a restore that did not happen.
func TestRestoreCommandRefusesATakenPlace(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("needs a POSIX shell")
	}
	dir := t.TempDir()
	from, to := filepath.Join(dir, "trash", "data"), filepath.Join(dir, "lib", "data")
	os.MkdirAll(from, 0o755)
	os.MkdirAll(filepath.Dir(to), 0o755)
	run := func() error { return exec.Command("sh", "-c", restoreCommand(from, to)).Run() }
	os.MkdirAll(to, 0o755)
	if err := run(); err == nil {
		t.Fatal("the undo reported success over an existing folder")
	}
	if _, err := os.Stat(filepath.Join(to, "data")); err == nil {
		t.Error("the old folder was put inside the new one")
	}
	os.Remove(to)
	if err := run(); err != nil {
		t.Fatalf("restore to a free place: %v", err)
	}
	if _, err := os.Stat(to); err != nil {
		t.Error("nothing was restored")
	}
}

// sh runs a command the way the changes are run.
func sh(t *testing.T, command string) error {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("needs a POSIX shell")
	}
	return exec.Command("sh", "-c", command).Run()
}

// Two things with one name moved to the Trash in the same second: the
// second move used to do nothing and report success.
func TestTrashPathIsFreeAndTheMoveIsChecked(t *testing.T) {
	old := Home
	Home = t.TempDir()
	t.Cleanup(func() { Home = old })
	trash := filepath.Join(Home, ".Trash")
	now := time.Date(2026, 11, 13, 10, 20, 30, 0, time.UTC)
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		src := filepath.Join(Home, "Applications", strconv.Itoa(i), "Foo.app")
		os.MkdirAll(src, 0o755)
		os.WriteFile(filepath.Join(src, "id"), []byte(strconv.Itoa(i)), 0o644)
		os.MkdirAll(trash, 0o755)
		dest := trashPath("Foo.app", now)
		if seen[dest] || filepath.Ext(dest) != ".app" {
			t.Fatalf("destination %d: %s", i, dest)
		}
		seen[dest] = true
		if err := sh(t, moveCommand(src, dest)); err != nil {
			t.Fatalf("move %d: %v", i, err)
		}
		if data, _ := os.ReadFile(filepath.Join(dest, "id")); string(data) != strconv.Itoa(i) {
			t.Errorf("%s holds %q", dest, data)
		}
	}
	if entries, _ := os.ReadDir(trash); len(entries) != 3 {
		t.Errorf("the Trash holds %d entries, want 3", len(entries))
	}

	// A destination taken after the plan was made: the command fails, and
	// nothing goes inside what is there.
	src := filepath.Join(Home, "Applications", "Bar.app")
	os.MkdirAll(src, 0o755)
	dest := trashPath("Bar.app", now)
	os.MkdirAll(dest, 0o755)
	if err := sh(t, moveCommand(src, dest)); err == nil {
		t.Error("a move onto a taken name reported success")
	}
	if _, err := os.Stat(src); err != nil {
		t.Error("the source is gone")
	}
	if entries, _ := os.ReadDir(dest); len(entries) != 0 {
		t.Error("the source was moved inside the folder that was there")
	}
}

// Leftovers moved halfway, and an undo that stopped halfway, can both be
// undone: what never left and what is back already are passed over, and
// what was lost is an error.
func TestLeftoverUndoAfterAPartialRun(t *testing.T) {
	old := Home
	Home = t.TempDir()
	t.Cleanup(func() { Home = old })
	lib := filepath.Join(Home, "Library")
	l := Leftover{Bundle: "com.gone.Editor", Paths: []string{
		filepath.Join(lib, "Application Support", "com.gone.Editor"),
		filepath.Join(lib, "Caches", "com.gone.Editor"),
		filepath.Join(lib, "Preferences", "com.gone.Editor.plist"),
	}}
	for _, p := range l.Paths[:2] {
		os.MkdirAll(p, 0o755)
	}
	os.MkdirAll(filepath.Join(Home, ".Trash"), 0o755)
	// The third path is missing: the command stops there, with two moved.
	c := l.TrashChange(time.Now())
	if err := sh(t, c.Command); err == nil {
		t.Fatal("moving a path that is not there reported success")
	}
	if _, err := os.Stat(l.Paths[0]); err == nil {
		t.Fatal("the first path was not moved")
	}
	if !c.KeepsUndoOnFailure() {
		t.Fatal("a trash that stopped halfway has to be journaled")
	}
	// The place of the second is taken again: the undo restores the first
	// and fails on the second.
	os.MkdirAll(filepath.Dir(l.Paths[2]), 0o755)
	os.WriteFile(l.Paths[2], []byte("x"), 0o644)
	os.MkdirAll(l.Paths[1], 0o755)
	if err := sh(t, c.Undo); err == nil {
		t.Fatal("an undo over a taken place reported success")
	}
	if _, err := os.Stat(l.Paths[0]); err != nil {
		t.Fatal("the first path was not restored")
	}
	// The place is freed: the same undo now finishes, passing over the
	// first (back already) and the third (never moved).
	os.Remove(l.Paths[1])
	if err := sh(t, c.Undo); err != nil {
		t.Fatalf("the undo did not finish on the second run: %v", err)
	}
	if _, err := os.Stat(l.Paths[1]); err != nil {
		t.Error("the second path was not restored")
	}
	// Nothing in the Trash and nothing in place: that is a loss, not "ok".
	os.RemoveAll(l.Paths[0])
	if err := sh(t, c.Undo); err == nil {
		t.Error("an undo with nothing to restore reported success")
	}
}

// The undo of a disabled job is as good as what launchd says afterwards.
func TestEnableUndoIsVerified(t *testing.T) {
	it := Item{Name: "Updater", Label: "com.vendor.updater", Kind: KindAgent, Enabled: true, Plist: "/Library/LaunchAgents/com.vendor.updater.plist"}
	undo := it.DisableChange().Undo
	if strings.HasSuffix(strings.TrimSpace(undo), "true") || !strings.Contains(undo, "launchctl print ") || !strings.Contains(undo, "/com.vendor.updater'") {
		t.Errorf("undo: %s", undo)
	}
	it.System, it.Kind = true, KindDaemon
	c := it.DisableChange()
	if !strings.Contains(c.Undo, "sudo launchctl bootstrap 'system'") || !strings.Contains(c.Undo, "launchctl print 'system/com.vendor.updater'") {
		t.Errorf("system undo: %s", c.Undo)
	}
}
