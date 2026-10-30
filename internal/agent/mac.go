package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/macos"
	"github.com/lcajigasm/omega/internal/safety"
)

func macToolSpecs() []llm.ToolSpec {
	return []llm.ToolSpec{
		{Name: "mac_inventory", Description: "What runs on this Mac outside the apps the user opens: items that open at login, jobs allowed " +
			"in the background, menu bar apps, the Dock and global keyboard shortcuts with their conflicts. Each entry carries omega's " +
			"verdict (keep, disable, review) from the user's rules (mac.toml) and omega's defaults, and the change ids to pass to mac_change.",
			Properties: map[string]any{"section": map[string]any{"type": "string", "enum": append([]string{"all"}, macos.Sections...)}}},
		{Name: "mac_change", Description: "Apply one change id from mac_inventory, e.g. disable:com.google.keystone.agent, quit:<bundle>, " +
			"dock-remove:<bundle>, dock-add:<app>, dock-set:autohide=true, hotkey-off:<id>, hotkey-set:<id>=<combo> (combos look like " +
			"opt+shift+space). Every change is reversible with `omega mac undo`. Some changes only open the right settings and return what " +
			"the user must do by hand: tell the user.",
			Properties: map[string]any{
				"change": str("change id from mac_inventory"),
				"reason": str("why, in one sentence; shown to the user"),
			}, Required: []string{"change", "reason"}},
		{Name: "mac_policy", Description: "Save a decision of the user in their rules (~/.config/omega/mac.toml) so omega suggests the same next " +
			"time. kind: keep or disable (login and background items), menubar-keep, menubar-hide, dock-remove, dock-add, or owner " +
			"(value like opt+space=ChatGPT, or opt+space=none). Values are app names, bundle ids or launchd labels; * is a wildcard.",
			Properties: map[string]any{
				"kind":  map[string]any{"type": "string", "enum": []string{"keep", "disable", "menubar-keep", "menubar-hide", "dock-remove", "dock-add", "owner"}},
				"value": str("pattern, app, or combo=owner"),
			}, Required: []string{"kind", "value"}},
	}
}

func (a *Agent) macInventory(section string) (*macos.Inventory, error) {
	if a.macInv != nil {
		return a.macInv, nil
	}
	p, err := macos.LoadPolicy()
	if err != nil {
		return nil, err
	}
	var sections []string
	if section != "" && section != "all" {
		sections = []string{section}
	}
	inv, err := macos.Load(p, sections...)
	if err != nil {
		return nil, err
	}
	if len(sections) == 0 {
		a.macInv = inv
	}
	return inv, nil
}

func toolMacInventory(_ context.Context, a *Agent, in map[string]any) (string, error) {
	section := getString(in, "section")
	a.emit(Event{Kind: EvToolStart, Tool: "mac_inventory", Command: "mac_inventory " + section, Risk: safety.Safe})
	start := time.Now()
	inv, err := a.macInventory(section)
	a.emit(Event{Kind: EvToolEnd, Tool: "mac_inventory", Command: "mac_inventory " + section, Failed: err != nil, Duration: time.Since(start)})
	if err != nil {
		return "", err
	}
	return inv.Report(section), nil
}

func toolMacChange(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	id := getString(in, "change")
	section, _, _ := strings.Cut(id, ":")
	if strings.HasPrefix(section, "hotkey") {
		section = "shortcuts"
	} else if strings.HasPrefix(section, "dock") {
		section = "dock"
	} else {
		section = ""
	}
	inv, err := a.macInventory(section)
	if err != nil {
		return "", err
	}
	c, err := inv.Resolve(id)
	if err != nil {
		return "", err
	}
	// The changes are journaled and reversible; only those that need sudo
	// count as dangerous.
	risk := safety.Normal
	if c.Sudo() {
		risk = safety.Dangerous
	}
	reason := c.Title
	if r := getString(in, "reason"); r != "" {
		reason += " · " + r
	}
	out, err := a.runShell(ctx, "mac_change", c.Command, reason, risk, false)
	if err != nil {
		return "", err
	}
	a.macInv = nil // the system changed: read it again next time
	if strings.HasPrefix(out, "exit_code: 0\n") {
		if err := macos.Record(c); err != nil {
			out += "\n(could not record the change for undo: " + err.Error() + ")"
		} else if c.Undo != "" {
			out += "\nRecorded: the user can revert it with `omega mac undo`."
		}
	}
	if c.Manual != "" {
		out += "\nThe user must finish this by hand: " + c.Manual
	}
	return out, nil
}

func toolMacPolicy(_ context.Context, a *Agent, in map[string]any) (string, error) {
	kind, value := getString(in, "kind"), getString(in, "value")
	p, err := macos.LoadPolicy()
	if err != nil {
		return "", err
	}
	if err := p.AddRule(kind, value); err != nil {
		return "", err
	}
	if err := p.Save(); err != nil {
		return "", err
	}
	a.macInv = nil
	a.emit(Event{Kind: EvToolEnd, Tool: "mac_policy", Command: i18n.T("agent.mac_rule", kind, value)})
	return fmt.Sprintf("saved in %s", macos.PolicyPath()), nil
}
