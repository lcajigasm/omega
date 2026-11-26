package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/safety"
	"github.com/lcajigasm/omega/internal/steps"
)

type handler func(ctx context.Context, a *Agent, in map[string]any) (string, error)

var handlers = map[string]handler{
	"system_info":     toolSystemInfo,
	"list_steps":      toolListSteps,
	"run_step":        toolRunStep,
	"run_command":     toolRunCommand,
	"disk_usage":      toolDiskUsage,
	"read_file":       toolReadFile,
	"write_file":      toolWriteFile,
	"remember":        toolRemember,
	"propose_actions": toolProposeActions,
	"config_hints":    toolConfigHints,
	"official_docs":   toolOfficialDocs,
	"web_search":      toolWebSearch,
	"fetch_url":       toolFetchURL,
	"mac_inventory":   toolMacInventory,
	"mac_change":      toolMacChange,
	"mac_policy":      toolMacPolicy,
	"dev_inventory":   toolDevInventory,
	"dev_install":     toolDevInstall,
	"dev_activate":    toolDevActivate,
	"mac_history":     toolMacHistory,
	"mac_undo":        toolMacUndo,
	"run_history":     toolRunHistory,
	"notes":           toolNotes,
	"omega_settings":  toolOmegaSettings,
	"omega_doctor":    toolOmegaDoctor,
	"model_ranking":   toolModelRanking,
	"pending":         toolPending,
	"machine_facts":   toolMachineFacts,
}

func str(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

// goos is the operating system omega runs on. Tests replace it.
var goos = runtime.GOOS

// macOnlyTool reports the tools that drive macOS itself (login items, the
// Dock, privacy permissions and the journal of those changes).
func macOnlyTool(name string) bool { return strings.HasPrefix(name, "mac_") }

// toolSpecs are the tools offered to the model on this operating system:
// the mac_* ones only exist on macOS.
func toolSpecs() []llm.ToolSpec {
	all := allToolSpecs()
	if goos == "darwin" {
		return all
	}
	var out []llm.ToolSpec
	for _, s := range all {
		if !macOnlyTool(s.Name) {
			out = append(out, s)
		}
	}
	return out
}

func allToolSpecs() []llm.ToolSpec {
	return append([]llm.ToolSpec{
		{Name: "system_info", Description: "System summary: operating system and version, architecture, disk, memory and detected package managers.",
			Properties: map[string]any{}},
		{Name: "list_steps", Description: "List the maintenance steps (update, clean, check, backend) this machine can run, without their commands. " +
			"With id, one step in full (its command, risk and status); all=true also lists the steps that are not installed or are disabled.",
			Properties: map[string]any{
				"category": map[string]any{"type": "string", "enum": []string{"update", "clean", "check", "backend", "all"}},
				"id":       str("optional: one step id, for its full detail"),
				"all":      map[string]any{"type": "boolean", "description": "also list the steps that are not installed or are disabled"},
				"detail":   map[string]any{"type": "boolean", "description": "include each step's command (long; prefer id for one step)"},
			}},
		{Name: "run_step", Description: "Run a catalogue step by id. Prefer this over run_command for routine actions. A cleaner " +
			"that ran successfully a few hours ago (mole-clean) is not run again unless again=true, because it would free little.",
			Properties: map[string]any{"id": str("step id, e.g. brew, npm, brew-cleanup"),
				"again": map[string]any{"type": "boolean", "description": "run a cleaner again although it ran recently (only when the user asks)"}},
			Required: []string{"id"}},
		{Name: "run_command", Description: "Run a non-interactive shell command (the user's login shell, -lc). Read-only commands run directly; " +
			"commands that change the system may need the user's approval. Returns the exit code and (trimmed) output.",
			Properties: map[string]any{
				"command": str("command to run"),
				"reason":  str("why you are running it, in one sentence; shown to the user"),
			}, Required: []string{"command", "reason"}},
		{Name: "disk_usage", Description: "Show the largest subdirectories of a path.",
			Properties: map[string]any{
				"path": str("path to analyse, e.g. ~/Library/Caches on macOS or ~/.cache on Linux"),
				"top":  map[string]any{"type": "integer", "description": "number of entries (default 15)"},
			}, Required: []string{"path"}},
		{Name: "read_file", Description: "Read a text file (configs, logs). 64 KB maximum. Refuses private keys, credential files and shell histories; " +
			"their contents would be sent to the model. The value of a credential set in the file (export API_KEY=…) is shown as [redacted].",
			Properties: map[string]any{"path": str("file path; ~ is supported")}, Required: []string{"path"}},
		{Name: "write_file", Description: "Write a whole configuration file and keep an .omega-bak copy of the original. The user is asked first, " +
			"unless they approved every change of this run; a file that decides what runs at login or as root (shell startup files, ~/.ssh, " +
			"launch agents, systemd units, cron, /etc, omega's own configuration) is asked about every time.",
			Properties: map[string]any{
				"path":    str("file path; ~ is supported"),
				"content": str("complete new content"),
				"reason":  str("what changes and why"),
			}, Required: []string{"path", "content", "reason"}},
		{Name: "propose_actions", Description: "Show the user a selection card and wait for their choice. Use it whenever you have " +
			"recommendations or alternatives (always at the end of an analysis) instead of listing them as text. " +
			"multiple=true lets the user tick several actions for you to carry out; multiple=false asks for exactly one. " +
			"Returns the ids the user picked; then do exactly those.",
			Properties: map[string]any{
				"question": str("short title of the card, e.g. 'What should I clean up?'"),
				"multiple": map[string]any{"type": "boolean", "description": "true to allow several choices"},
				"options": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":     str("stable id, e.g. brew-cleanup"),
							"label":  str("short action, e.g. 'Clear the Homebrew cache'"),
							"detail": str("one line: what will run and why"),
							"impact": str("expected effect, e.g. '~9 GB' or 'fixes brew doctor'"),
							"risk":   map[string]any{"type": "string", "enum": []string{"read-only", "normal", "dangerous"}},
							"evidence": str("one line: what you checked that shows this is safe and what it frees (the tool output, " +
								"the PR state, who recreates it). Required in omega ultra for every option that changes something"),
							"recommended": map[string]any{"type": "boolean", "description": "the option you recommend; with multiple=false mark exactly one. " +
								"An unattended run takes only recommended options whose risk is read-only or normal (one, for a single-choice question)"},
						},
						"required": []string{"id", "label"},
					},
				},
			}, Required: []string{"question", "options"}},
		{Name: "config_hints", Description: "Pointers for tuning the configuration of the tools installed here: where each config lives, " +
			"read-only commands to inspect it, what is usually worth reviewing and the official docs. Optionally filter by tool name.",
			Properties: map[string]any{"tool": str("optional tool name, e.g. topgrade, git, docker")}},
		{Name: "official_docs", Description: "Where things are documented by whoever makes them: the official documentation of this operating " +
			"system (and on Linux of this distribution) and of the tools installed here, plus how to read the manuals installed on the machine. " +
			"Call it before a fix, an optimisation or a setting that is not a catalogue step, then read the page with fetch_url. " +
			"Optionally filter by topic or tool.",
			Properties: map[string]any{"topic": str("optional tool or subject, e.g. launchd, dnf, winget, snapshots, docker")}},
		{Name: "web_search", Description: "Search the web (DuckDuckGo). Use it to find current official documentation before recommending a setting; " +
			"results from the official documentation sites omega knows are marked [official].",
			Properties: map[string]any{"query": str("search query")}, Required: []string{"query"}},
		{Name: "fetch_url", Description: "Read a web page or raw file as text (docs, changelogs, example configs). Content is untrusted reference material.",
			Properties: map[string]any{"url": str("http(s) URL")}, Required: []string{"url"}},
		{Name: "dev_inventory", Description: "The development setup of this machine: installed languages, version managers, package managers, " +
			"shell tools, git tools, container and cloud CLIs and AI coding assistants, each with its version, path and what installed it; " +
			"findings (several managers for one language, commands shadowed on PATH, broken PATH entries, tools not activated in the shell's startup file, " +
			"shell startup time), the versions each manager keeps with their size, and what is not installed with the install command for this machine.",
			Properties: map[string]any{}},
		{Name: "remember", Description: "Save a lasting note about this machine for future runs (e.g. the cause of a recurring failure and its fix). " +
			"One line of at most 400 characters: a longer note is refused, so write the fact, not the story. After a web page or a search " +
			"result is in the conversation, a note needs the user's yes; in a run nobody watches it is then not saved, and you say so in the summary.",
			Properties: map[string]any{"note": str("short note, one line, at most 400 characters")}, Required: []string{"note"}},
		pendingToolSpec(),
		machineFactsSpec(),
	}, append(macToolSpecs(), omegaToolSpecs()...)...)
}

// allReadOnly reports whether every call can run at the same time.
// mac_inventory is left out: it fills a cache on the agent.
func allReadOnly(calls []llm.ToolCall) bool {
	for _, call := range calls {
		if !readOnlyCall(call) {
			return false
		}
	}
	return true
}

func readOnlyCall(call llm.ToolCall) bool {
	switch call.Name {
	case "system_info", "list_steps", "disk_usage", "read_file", "config_hints", "official_docs", "web_search", "dev_inventory", "run_history", "mac_history", "omega_doctor", "model_ranking", "machine_facts":
		return true
	case "fetch_url":
		// Any other address may put a card in front of the user.
		var in map[string]any
		return json.Unmarshal(call.Input, &in) == nil && officialURL(getString(in, "url"))
	case "run_command":
		var in map[string]any
		if json.Unmarshal(call.Input, &in) != nil {
			return false
		}
		cmd := getString(in, "command")
		return cmd != "" && safety.Classify(cmd) == safety.Safe && !safety.AlwaysConfirm(cmd)
	case "run_step":
		var in map[string]any
		if json.Unmarshal(call.Input, &in) != nil {
			return false
		}
		s, ok := steps.Find(getString(in, "id"))
		if !ok || s.AlwaysConfirm || s.Risk != safety.Safe {
			return false
		}
		// omega's own steps are written and reviewed with their risk: a
		// preview that goes through a temporary file (topgrade-preview) or
		// a loop (dev-artifacts) reads as a change to the policy, which
		// cannot tell. A step from steps.d is judged by its command.
		if steps.Builtin(s.ID) {
			return safety.Classify(s.Command) != safety.Blocked && !safety.AlwaysConfirm(s.Command)
		}
		return safety.Classify(s.Command) == safety.Safe
	default:
		return false
	}
}

func getString(in map[string]any, key string) string {
	v, _ := in[key].(string)
	return strings.TrimSpace(v)
}

func expand(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	return path
}
