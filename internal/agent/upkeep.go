package agent

import (
	"errors"

	"github.com/lcajigasm/omega/internal/llm"
)

// What an unattended run does not do. It is upkeep: it checks the machine,
// updates what is installed and clears caches. Everything else changes how
// the user set the machine up, and people found it done behind their back:
// apps gone from what starts at login, icons gone from the menu bar. Those
// are a person's decisions, so the run has no tool for them and refuses
// the steps and commands that would make them.

// notUpkeepTools are the tools an unattended run does not get, with what
// each would change.
var notUpkeepTools = map[string]string{
	"write_file":     "a configuration file",
	"mac_change":     "what runs on the Mac (login and background items, the menu bar, the Dock, shortcuts, apps, permissions)",
	"mac_policy":     "the rules in mac.toml",
	"mac_undo":       "a change the user made with omega mac",
	"dev_install":    "the installed language versions",
	"dev_activate":   "the active language versions",
	"omega_settings": "omega's own settings",
}

// notUpkeepSteps are the catalogue steps an unattended run does not run,
// with why.
var notUpkeepSteps = map[string]string{
	"trash": "emptying the Trash deletes the user's files for good",
	"mole-optimize": "Mole's optimize repairs login items, background services, preferences and the saved windows of apps, " +
		"which is how the user set up the Mac",
	"brew-greedy": "it upgrades the apps that update themselves, quitting them while they are open",
	"topgrade": "topgrade upgrades every app with its own steps, which quit open apps and remove login items without " +
		"omega's checks; the catalogue steps do the same upkeep with them",
}

// upkeepTools leaves out of specs the tools an unattended run does not get.
func upkeepTools(specs []llm.ToolSpec) []llm.ToolSpec {
	var out []llm.ToolSpec
	for _, s := range specs {
		if _, off := notUpkeepTools[s.Name]; !off {
			out = append(out, s)
		}
	}
	return out
}

// notUpkeep refuses, in an unattended run, a tool or a step that is not
// upkeep; nil when it may go ahead.
func (a *Agent) notUpkeep(tool, command, what string) error {
	why := "Unattended run: " + what + ". That is how the user set up this machine, not upkeep, so it was left as it is. " +
		"Do not retry it or an equivalent command; put it in your summary as a suggestion the user can apply themselves."
	a.emit(Event{Kind: EvDenied, Tool: tool, Command: command, Reason: why})
	return explained(errors.New(why))
}
