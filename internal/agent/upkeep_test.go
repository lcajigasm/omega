package agent

import (
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/llm"
)

// An unattended run is upkeep: it has no tool to change how the user set
// up the machine, refuses the steps and the commands that would, and still
// updates and cleans.
func TestUnattendedIsUpkeepOnly(t *testing.T) {
	for _, spec := range upkeepTools(toolSpecs()) {
		if _, off := notUpkeepTools[spec.Name]; off {
			t.Errorf("an unattended run is offered %s", spec.Name)
		}
	}
	names := map[string]bool{}
	for _, spec := range upkeepTools(toolSpecs()) {
		names[spec.Name] = true
	}
	for _, keep := range []string{"run_step", "run_command", "propose_actions", "system_info", "pending"} {
		if !names[keep] {
			t.Errorf("an unattended run lost %s", keep)
		}
	}

	opts := Options{Unattended: true, AutoYes: true, SkipDetect: true}
	s := runScript(t, opts, Approve, nil, nil,
		&llm.Reply{Calls: []llm.ToolCall{
			call("1", "run_command", map[string]any{"command": "killall Bartender", "reason": "r"}),
			call("2", "run_command", map[string]any{"command": `osascript -e 'tell application "System Events" to delete login item "Alfred 5"'`, "reason": "r"}),
			call("3", "run_command", map[string]any{"command": "brew uninstall --cask android-studio", "reason": "r"}),
			call("4", "run_step", map[string]any{"id": "trash"}),
			call("5", "run_step", map[string]any{"id": "mole-optimize"}),
			call("6", "write_file", map[string]any{"path": "/tmp/x.toml", "content": "x", "reason": "r"}),
			call("7", "mac_change", map[string]any{"change": "quit:com.surteesstudios.Bartender"}),
			call("8", "run_command", map[string]any{"command": "brew upgrade --formula", "reason": "r"}),
			call("9", "run_step", map[string]any{"id": "npm"}),
		}})
	for i, r := range s.p.results[1][:7] {
		if !r.IsError || !strings.Contains(r.Content, "Unattended run") {
			t.Errorf("call %d was not refused: %q", i+1, r.Content)
		}
	}
	if len(s.ran) != 2 || s.ran[0] != "brew upgrade --formula" || !strings.Contains(s.ran[1], "npm update -g") {
		t.Errorf("only the upkeep must run: %q", s.ran)
	}

	// With someone there, the same commands are put to the approval policy
	// as always.
	s = runScript(t, Options{AutoYes: true}, Approve, nil, nil,
		&llm.Reply{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{"command": "brew uninstall --cask android-studio", "reason": "r"})}})
	if len(s.ran) != 1 {
		t.Errorf("an attended run with --yes runs what the user approved: %q", s.ran)
	}
}
