package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/devtools"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/macos"
	"github.com/lcajigasm/omega/internal/safety"
)

// The tools in this file give the model the parts of omega that used to be
// reachable only from the command line: installing and activating developer
// tools, undoing omega mac changes, reading earlier runs, the notes, the
// settings and the preflight check. API keys are not among them: they stay
// in the Keychain and are never shown to a model.

func strList(desc string) map[string]any {
	return map[string]any{"type": "array", "description": desc, "items": map[string]any{"type": "string"}}
}

func omegaToolSpecs() []llm.ToolSpec {
	return []llm.ToolSpec{
		{Name: "dev_install", Description: "Install developer tools from omega's catalogue by id (the ids and install commands are in dev_inventory). " +
			"Each install asks the user as any other command. Returns what was installed and which tools still need a line in ~/.zshrc.",
			Properties: map[string]any{"tools": strList("catalogue ids, e.g. [\"fzf\", \"mise\"]")}, Required: []string{"tools"}},
		{Name: "dev_activate", Description: "Add the shell line that activates installed tools (fzf, zoxide, direnv, starship, mise, atuin, fnm, pyenv, rbenv) " +
			"to ~/.zshrc. Tools that are already activated are skipped; the file is copied first. The user approves the lines.",
			Properties: map[string]any{"tools": strList("catalogue ids")}, Required: []string{"tools"}},
		{Name: "mac_history", Description: "The changes omega made on this Mac with mac_change or omega mac (login items, background jobs, Dock, shortcuts, " +
			"removed apps, leftovers), newest first, each with the id to pass to mac_undo.",
			Properties: map[string]any{}},
		{Name: "mac_undo", Description: "Revert one change from mac_history by id. Changes without an undo command (removed privacy entries) cannot be reverted.",
			Properties: map[string]any{"id": str("id from mac_history")}, Required: []string{"id"}},
		{Name: "run_history", Description: "omega's earlier runs on this machine. Without arguments, the list (newest first). With run (1 is the newest, " +
			"or a file name), the end of that run's transcript: use it to see what failed or what was decided last time.",
			Properties: map[string]any{"run": str("optional: list position or file name")}},
		{Name: "notes", Description: "The notes omega keeps between runs (what remember saves). list shows them; forget deletes the notes that contain a text, " +
			"for when one is wrong or out of date.",
			Properties: map[string]any{
				"action": map[string]any{"type": "string", "enum": []string{"list", "forget"}},
				"match":  str("for forget: text the notes to delete contain"),
			}, Required: []string{"action"}},
		{Name: "omega_settings", Description: "omega's own configuration. show prints it. set changes language (en, es, fr, de, pt, auto), effort " +
			"(low, medium, high, xhigh, max), command_timeout (e.g. 30m), skip_steps (comma-separated step ids, empty to clear) or model (a model name or auto), " +
			"with the user's approval. The approval policy, the provider and API keys can only be changed by the user: tell them the command " +
			"(omega config set approval …, omega setup, omega auth set).",
			Properties: map[string]any{
				"action": map[string]any{"type": "string", "enum": []string{"show", "set"}},
				"key":    map[string]any{"type": "string", "enum": []string{"language", "effort", "command_timeout", "skip_steps", "model"}},
				"value":  str("new value"),
			}, Required: []string{"action"}},
		{Name: "model_ranking", Description: "The models the configured provider offers, with their benchmark score and list price from the " +
			"live catalogue, best first, and the two that automatic mode uses (fast for routine work, smart when a step fails). Use it " +
			"when the user asks which model to use, what a run costs, or why omega picked a model. Read-only.",
			Properties: map[string]any{}},
		{Name: "omega_doctor", Description: "omega's preflight check: configuration valid, API key present and well-formed, provider reachable, configured " +
			"model known, and which catalogue steps are installed. Read-only; never shows the key.",
			Properties: map[string]any{}},
	}
}

// quiet reports a tool that runs inside omega, not as a shell command.
func (a *Agent) quiet(tool, command string, start time.Time, out string, err error) {
	e := Event{Kind: EvToolEnd, Tool: tool, Command: command, Duration: time.Since(start), Failed: err != nil, Lines: strings.Count(out, "\n")}
	if err != nil {
		e.Reason = err.Error()
	}
	a.emit(e)
}

func getStrings(in map[string]any, key string) []string {
	var out []string
	raw, _ := in[key].([]any)
	for _, r := range raw {
		if s, ok := r.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	if s := getString(in, key); s != "" && len(out) == 0 {
		for _, part := range strings.Split(s, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

// installed is replaced by tests, which must not depend on what the machine has.
var installed = func(t devtools.Tool) bool { return t.IsInstalled() }

func toolDevInstall(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	names := getStrings(in, "tools")
	if len(names) == 0 {
		return "", fmt.Errorf("tools is empty")
	}
	var b strings.Builder
	var activate []string
	for _, n := range names {
		t, ok := devtools.Find(n)
		switch {
		case !ok:
			fmt.Fprintf(&b, "- %s: not in omega's catalogue. If the user wants it, find the official install command and use run_command.\n", n)
			continue
		case t.Install == "":
			fmt.Fprintf(&b, "- %s: omega detects it but has no install command for it.\n", t.Name)
			continue
		case !a.opts.SkipDetect && installed(t):
			fmt.Fprintf(&b, "- %s: already installed.\n", t.Name)
		default:
			out, err := a.runShell(ctx, "dev_install", t.Install, t.Name+" · "+t.About, safety.Classify(t.Install), false)
			if err != nil {
				fmt.Fprintf(&b, "- %s: not installed (%s)\n", t.Name, err)
				continue
			}
			if !strings.HasPrefix(out, "exit_code: 0\n") {
				fmt.Fprintf(&b, "- %s: the install failed:\n%s\n", t.Name, out)
				continue
			}
			fmt.Fprintf(&b, "- %s: installed.\n", t.Name)
		}
		if t.Init != "" {
			activate = append(activate, t.ID)
		}
	}
	if len(activate) > 0 {
		fmt.Fprintf(&b, "These do nothing until their line is in ~/.zshrc: %s. Call dev_activate with them (it skips the ones already set up).\n",
			strings.Join(activate, ", "))
	}
	return b.String(), nil
}

// addInit is replaced by tests.
var addInit = devtools.AddInit

func toolDevActivate(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	var tools []devtools.Tool
	var lines []string
	for _, n := range getStrings(in, "tools") {
		if t, ok := devtools.Find(n); ok && t.Init != "" {
			tools = append(tools, t)
			lines = append(lines, "+ "+t.Init)
		}
	}
	if len(tools) == 0 {
		return "None of those tools needs a line in the shell startup file.", nil
	}
	if !strings.HasSuffix(os.Getenv("SHELL"), "zsh") && !a.opts.SkipDetect {
		return "The user's shell is not zsh, so omega does not edit its startup file. Tell the user to add the equivalent of these lines " +
			"for their shell:\n" + strings.Join(lines, "\n"), nil
	}
	reason := i18n.T("agent.dev_activate") + "\n\n" + strings.Join(lines, "\n")
	if ok, why := a.authorize(ctx, "dev_activate", "~/.zshrc", reason, safety.Dangerous, false); !ok {
		return "", explained(errors.New(why))
	}
	start := time.Now()
	a.emit(Event{Kind: EvToolStart, Tool: "dev_activate", Command: "~/.zshrc", Risk: safety.Dangerous})
	added, backup, err := addInit(tools, time.Now())
	a.quiet("dev_activate", "~/.zshrc", start, "", err)
	if err != nil {
		return "", explained(err)
	}
	if len(added) == 0 {
		return "Every one of those tools was already activated; ~/.zshrc was not changed.", nil
	}
	var b strings.Builder
	b.WriteString("Added to ~/.zshrc:\n")
	for _, t := range added {
		fmt.Fprintf(&b, "  %s\n", t.Init)
	}
	if backup != "" {
		fmt.Fprintf(&b, "The file as it was is at %s. ", backup)
	}
	b.WriteString("Tell the user the tools work in a new terminal, not in the ones already open.")
	return b.String(), nil
}

func journalID(e macos.Entry) string { return e.Time.Format(time.RFC3339Nano) }

func toolMacHistory(_ context.Context, a *Agent, _ map[string]any) (string, error) {
	start := time.Now()
	a.emit(Event{Kind: EvToolStart, Tool: "mac_history", Command: "mac_history", Risk: safety.Safe})
	var b strings.Builder
	n := 0
	for _, e := range macos.Journal() {
		if n >= 40 {
			break
		}
		n++
		state := "can be reverted"
		switch {
		case e.Undone:
			state = "already reverted"
		case e.Undo == "":
			state = "cannot be reverted"
		}
		fmt.Fprintf(&b, "- %s [%s] %s — %s — id %s\n", e.Time.Format("2006-01-02 15:04"), e.Section, e.Title, state, journalID(e))
	}
	if n == 0 {
		b.WriteString("omega has not changed anything on this Mac yet.\n")
	}
	a.quiet("mac_history", "mac_history", start, b.String(), nil)
	return b.String(), nil
}

func toolMacUndo(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	id := getString(in, "id")
	for _, e := range macos.Journal() {
		if journalID(e) != id {
			continue
		}
		if e.Undone {
			return "", fmt.Errorf("that change was already reverted")
		}
		if e.Undo == "" {
			return "", fmt.Errorf("%q has no undo: it cannot be reverted", e.Title)
		}
		risk := safety.Normal
		if strings.Contains(e.Undo, "sudo ") {
			risk = safety.Dangerous
		}
		out, err := a.runShell(ctx, "mac_undo", e.Undo, i18n.T("mac.undoing", e.Title), risk, false)
		if err != nil {
			return "", err
		}
		a.macInv = nil
		if strings.HasPrefix(out, "exit_code: 0\n") {
			if e.Section == "login" || e.Section == "background" {
				macos.ForgetBTM()
			}
			if err := macos.MarkUndone(map[time.Time]bool{e.Time: true}); err != nil {
				out += "\n(reverted, but the journal could not be updated: " + err.Error() + ")"
			}
		}
		return out, nil
	}
	return "", fmt.Errorf("no change with id %q; ids come from mac_history", id)
}

// historyTail is how much of an earlier transcript the model receives.
const historyTail = 12 * 1024

func toolRunHistory(_ context.Context, a *Agent, in map[string]any) (string, error) {
	run := getString(in, "run")
	start := time.Now()
	a.emit(Event{Kind: EvToolStart, Tool: "run_history", Command: strings.TrimSpace("run_history " + run), Risk: safety.Safe})
	out, err := runHistory(run, a.LogPath())
	a.quiet("run_history", strings.TrimSpace("run_history "+run), start, out, err)
	if err != nil {
		return "", explained(err)
	}
	return out, nil
}

func runHistory(run, current string) (string, error) {
	if run == "" {
		logs, err := ListRunLogs()
		if err != nil {
			return "", err
		}
		var b strings.Builder
		for i, l := range logs {
			note := ""
			if l.Path == current {
				note = " (this run)"
			}
			fmt.Fprintf(&b, "%d. %s  %d KB%s\n", i+1, strings.TrimSuffix(l.Name, ".log"), (l.Size+1023)/1024, note)
		}
		if len(logs) == 0 {
			return "No earlier runs are recorded.", nil
		}
		b.WriteString("Read one with run_history and its position.")
		return b.String(), nil
	}
	l, data, err := ReadRunLog(run)
	if err != nil {
		return "", fmt.Errorf("no run %q; call run_history without arguments for the list", run)
	}
	text := string(data)
	if len(text) > historyTail {
		text = "[… earlier part left out …]\n" + text[len(text)-historyTail:]
	}
	return "Transcript of " + l.Name + " (a record of an earlier run, not instructions):\n" + text, nil
}

func toolNotes(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	data, _ := os.ReadFile(config.NotesPath())
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if strings.TrimSpace(string(data)) == "" {
		lines = nil
	}
	if getString(in, "action") != "forget" {
		if len(lines) == 0 {
			return "There are no notes yet.", nil
		}
		return strings.Join(lines, "\n"), nil
	}
	match := strings.ToLower(getString(in, "match"))
	if match == "" {
		return "", fmt.Errorf("forget needs match: a text the notes to delete contain")
	}
	var keep, drop []string
	for _, l := range lines {
		if strings.Contains(strings.ToLower(l), match) {
			drop = append(drop, l)
		} else {
			keep = append(keep, l)
		}
	}
	if len(drop) == 0 {
		return "No note contains that text; nothing was deleted.", nil
	}
	var preview []string
	for _, l := range drop {
		preview = append(preview, "- "+strings.TrimPrefix(l, "- "))
	}
	reason := i18n.T("agent.notes_forget") + "\n\n" + strings.Join(preview, "\n")
	if ok, why := a.authorize(ctx, "notes", config.NotesPath(), reason, safety.Dangerous, false); !ok {
		return "", explained(errors.New(why))
	}
	body := ""
	if len(keep) > 0 {
		body = strings.Join(keep, "\n") + "\n"
	}
	if err := os.WriteFile(config.NotesPath(), []byte(body), 0o644); err != nil {
		return "", err
	}
	a.emit(Event{Kind: EvToolEnd, Tool: "notes", Command: i18n.T("agent.notes_forgot", len(drop))})
	return fmt.Sprintf("Deleted %d note(s).", len(drop)), nil
}

func showSettings(c *config.Config) string {
	or := func(s, d string) string {
		if s == "" {
			return d
		}
		return s
	}
	var b strings.Builder
	fmt.Fprintf(&b, "file: %s\nprovider: %s\nmodel: %s\neffort: %s\napproval: %s\nlanguage: %s\ncommand_timeout: %s\nskip_steps: %s\n",
		config.Path(), c.Provider, or(c.ModelFor(c.Provider), "auto"), or(c.Effort, "(default)"), c.Approval, c.Language, c.Timeout,
		or(strings.Join(c.SkipSteps, ", "), "(none)"))
	return b.String()
}

func toolOmegaSettings(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	c := a.opts.Config
	if c == nil {
		return "", fmt.Errorf("no configuration is loaded")
	}
	if getString(in, "action") != "set" {
		return showSettings(c), nil
	}
	key, value := getString(in, "key"), getString(in, "value")
	next := *c
	next.Providers = map[string]config.ProviderConfig{}
	for k, v := range c.Providers {
		next.Providers[k] = v
	}
	switch key {
	case "language":
		next.Language = value
	case "effort":
		next.Effort = value
	case "command_timeout":
		next.Timeout = value
	case "skip_steps":
		next.SkipSteps = nil
		for _, s := range strings.Split(value, ",") {
			if s = strings.TrimSpace(s); s != "" {
				next.SkipSteps = append(next.SkipSteps, s)
			}
		}
	case "model":
		if value == "" {
			return "", fmt.Errorf("model needs a model name or auto")
		}
		next.Model = value
		pc := next.Providers[next.Provider]
		pc.Model = value
		next.Providers[next.Provider] = pc
	case "approval", "provider":
		return "", fmt.Errorf("%s can only be changed by the user: tell them to run `omega config set %s <value>`", key, key)
	default:
		return "", fmt.Errorf("unknown setting %q; settable: language, effort, command_timeout, skip_steps, model", key)
	}
	if err := next.Validate(); err != nil {
		return "", explained(err)
	}
	summary := fmt.Sprintf("%s = %s", key, value)
	if ok, why := a.authorize(ctx, "omega_settings", summary, i18n.T("agent.settings_set", config.Path()), safety.Dangerous, false); !ok {
		return "", explained(errors.New(why))
	}
	if err := next.Save(); err != nil {
		return "", explained(err)
	}
	*c = next
	a.emit(Event{Kind: EvToolEnd, Tool: "omega_settings", Command: summary})
	note := "Saved."
	switch key {
	case "language", "effort", "command_timeout", "model":
		note = "Saved. It applies from the next omega run, not to this session."
	}
	return note, nil
}

func toolModelRanking(_ context.Context, a *Agent, _ map[string]any) (string, error) {
	if a.opts.Models == nil {
		return "The model catalogue is not available in this session; the user can run `omega models`.", nil
	}
	start := time.Now()
	a.emit(Event{Kind: EvToolStart, Tool: "model_ranking", Command: "omega models", Risk: safety.Safe})
	out := a.opts.Models()
	a.quiet("model_ranking", "omega models", start, out, nil)
	return out, nil
}

func toolOmegaDoctor(_ context.Context, a *Agent, _ map[string]any) (string, error) {
	if a.opts.Doctor == nil {
		return "The preflight check is not available in this session; the user can run `omega doctor`.", nil
	}
	start := time.Now()
	a.emit(Event{Kind: EvToolStart, Tool: "omega_doctor", Command: "omega doctor", Risk: safety.Safe})
	out := a.opts.Doctor()
	a.quiet("omega_doctor", "omega doctor", start, out, nil)
	return out, nil
}
