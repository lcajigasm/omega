package macos

import (
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/i18n"
)

// An app that once asked to open at login leaves a switched-off record with
// the app's bundle id, and its "Allow in the Background" entry has the same
// id. Disabling that id must reach the entry that is on, not ask System
// Events to delete a login item that does not exist.
func TestDisableReachesTheItemThatIsOn(t *testing.T) {
	i18n.Init("en")
	inv := &Inventory{Items: []Item{
		{Name: "ChatGPT", Bundle: "com.openai.codex", Kind: KindLogin, Enabled: false, Path: "/Applications/ChatGPT.app"},
		{Name: "Shottr", Bundle: "cc.ffitch.shottr", Kind: KindLogin, Enabled: true, Path: "/Applications/Shottr.app"},
		{Name: "ChatGPT - background tasks", Bundle: "com.openai.codex", Kind: KindTask, Enabled: true},
		{Name: "GoogleUpdater", Label: "com.google.GoogleUpdater.wake", Kind: KindAgent, Enabled: true},
	}}
	c, err := inv.Resolve("disable:com.openai.codex")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(c.Command, "osascript") || !strings.Contains(c.Command, PaneLoginItems) || c.Manual == "" {
		t.Errorf("the background entry opens the pane and says what to do by hand: %+v", c)
	}
	// By name too, and a record that is off is never "removed" again.
	if _, err := inv.Resolve("disable:ChatGPT"); err == nil {
		t.Error("ChatGPT does not open at login: there is nothing to disable under that name")
	}
	// A real login item: System Events first, and when the app registered
	// itself, the pane and a sentence instead of an AppleScript error.
	c, err = inv.Resolve("disable:cc.ffitch.shottr")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`delete login item "Shottr"`, "|| {", PaneLoginItems, "Shottr adds itself"} {
		if !strings.Contains(c.Command, want) {
			t.Errorf("login item command lacks %q: %s", want, c.Command)
		}
	}
	c, _ = inv.Resolve("disable:com.google.GoogleUpdater.wake")
	if !strings.Contains(c.Command, "launchctl disable") {
		t.Errorf("a launchd job is still disabled with launchctl: %s", c.Command)
	}
	report := inv.Report("background")
	if !strings.Contains(report, "ChatGPT - background tasks") || !strings.Contains(report, "MANUAL ONLY: change disable:com.openai.codex") {
		t.Errorf("the report must say which items only the user can switch:\n%s", report)
	}
	if strings.Contains(report, "GoogleUpdater (agent, on)  [com.google.GoogleUpdater.wake]  verdict= category= — MANUAL") {
		t.Errorf("a launchd job is not manual:\n%s", report)
	}
}

// What is left in the system-wide privacy list can only be removed in System
// Settings: the change opens the pane and says so, and never fails.
func TestSystemWidePermissionsAreManual(t *testing.T) {
	i18n.Init("en")
	gone := Permission{Service: "kTCCServiceAccessibility", Client: "com.openai.chat", Name: "com.openai.chat", System: true, Missing: true}
	c := gone.ResetChange()
	if strings.Contains(c.Command, "perm-reset") || !strings.Contains(c.Command, "x-apple.systempreferences") || !strings.Contains(c.Manual, "press –") {
		t.Errorf("system-wide leftover: %+v", c)
	}
	mine := Permission{Service: "kTCCServiceCamera", Client: "/gone/node", Name: "node", IsPath: true, Missing: true}
	if c := mine.ResetChange(); !strings.Contains(c.Command, "mac op perm-reset") || c.Manual != "" {
		t.Errorf("the user's own database is still edited by omega: %+v", c)
	}
	installed := Permission{Service: "kTCCServiceAccessibility", Client: "com.example.app", System: true}
	if installed.ManualOnly() {
		t.Error("tccutil can reset an installed app")
	}
}

func TestGitMaintenanceIsKept(t *testing.T) {
	var p UserPolicy
	if v, _, _ := p.StartupVerdict("git", "", "org.git-scm.git.hourly"); v != Keep {
		t.Errorf("git maintenance: %s", v)
	}
}
