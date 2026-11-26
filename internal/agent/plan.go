package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/deepclean"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/safety"
	"github.com/lcajigasm/omega/internal/steps"
)

// omega ultra runs in two stages. The audit looks at everything every
// stage of omega covers, changes nothing, and turns each recommendation
// into a planned action: what was found, what would be done, what it rests
// on and the exact tool calls that do it. The person reviews the plan once,
// ticks what they want, and omega carries out exactly those calls, with no
// model and no further questions: the tick is the approval.

// PlanStep is one tool call of a planned action, exactly as the model would
// have made it.
type PlanStep struct {
	Tool  string         `json:"tool"`
	Input map[string]any `json:"input"`
	// Show is the step as the person reads it in the review: the command
	// that runs, the folder that goes.
	Show string `json:"show"`
}

// Action statuses after the plan was carried out.
const (
	ActionPlanned = ""
	ActionDone    = "done"
	ActionFailed  = "failed"
)

// PlannedAction is one change the audit recommends.
type PlannedAction struct {
	ID string `json:"id"`
	// Stage is the audit stage that found it (the task: clean, ultra-deep…).
	Stage    string `json:"stage"`
	Category string `json:"category,omitempty"`
	Title    string `json:"title"`
	// Why is what the audit found, in plain words with its figures.
	Why string `json:"why"`
	// What is what happens if it is ticked, and whether it can be undone.
	What        string `json:"what"`
	Impact      string `json:"impact,omitempty"`
	Bytes       int64  `json:"bytes,omitempty"`
	Risk        string `json:"risk"`
	Evidence    string `json:"evidence"`
	Recommended bool   `json:"recommended,omitempty"`
	Reversible  bool   `json:"reversible,omitempty"`
	Steps       []PlanStep
	// Findings are the deep_scan findings its deep_clean steps remove, as
	// the scan saw them: they are checked again just before.
	Findings []deepclean.Finding `json:"findings,omitempty"`
	Status   string              `json:"status,omitempty"`
	Result   string              `json:"result,omitempty"`
}

// Plan is what an audit recommends. The stages of one audit add to the same
// plan, one after another.
type Plan struct {
	mu      sync.Mutex
	Created time.Time       `json:"created"`
	Host    string          `json:"host,omitempty"`
	Actions []PlannedAction `json:"actions"`
}

// NewPlan starts an empty plan.
func NewPlan() *Plan {
	h, _ := os.Hostname()
	return &Plan{Created: time.Now(), Host: h}
}

// List is a copy of the actions, in the order they were found.
func (p *Plan) List() []PlannedAction {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]PlannedAction(nil), p.Actions...)
}

// Count is how many actions the plan has and how much space they would
// free, as far as the audit measured it.
func (p *Plan) Count() (n int, bytes int64) {
	if p == nil {
		return 0, 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, a := range p.Actions {
		n++
		bytes += a.Bytes
	}
	return n, bytes
}

// Mark records how an action ended when it was carried out.
func (p *Plan) Mark(id, status, result string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.Actions {
		if p.Actions[i].ID == id {
			p.Actions[i].Status, p.Actions[i].Result = status, result
		}
	}
}

// add puts an action in the plan; one with the same id replaces it.
func (p *Plan) add(act PlannedAction) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.Actions {
		if p.Actions[i].ID == act.ID {
			p.Actions[i] = act
			return
		}
	}
	p.Actions = append(p.Actions, act)
}

// PlanPath is where the last audit's plan is kept.
func PlanPath() string { return filepath.Join(config.StateDir(), "ultra-plan.json") }

// Save writes the plan, for this user only.
func (p *Plan) Save() error {
	p.mu.Lock()
	b, err := json.MarshalIndent(p, "", "  ")
	p.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(config.StateDir(), 0o700); err != nil {
		return err
	}
	tmp := PlanPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, PlanPath())
}

// ErrNoPlan is LoadPlan with no plan kept.
var ErrNoPlan = errors.New("no audit plan is kept")

// LoadPlan reads the last audit's plan.
func LoadPlan() (*Plan, error) {
	b, err := os.ReadFile(PlanPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoPlan
	}
	if err != nil {
		return nil, err
	}
	var p Plan
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("reading %s: %w", PlanPath(), err)
	}
	return &p, nil
}

// promptSummary is what the next audit stage is told about the plan so far,
// so it does not plan the same thing twice.
func (p *Plan) PromptSummary() string {
	acts := p.List()
	if len(acts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nActions already in the plan from earlier audit stages (do not plan them again; reuse an id only to replace one with better evidence):\n")
	for _, a := range acts {
		fmt.Fprintf(&b, "- %s: %s\n", a.ID, a.Title)
	}
	return b.String()
}

// AuditSuffix tells the model the stage is part of omega ultra's audit.
const AuditSuffix = "\n\nULTRA AUDIT (omega ultra, stage 1 of 2). Nothing changes during the audit: this session is read-only and " +
	"nobody is watching it. Investigate everything this stage covers as deeply as omega ultra asks, then turn EVERY change you " +
	"recommend into an action with plan_actions. Wherever the task says propose_actions, apply, carry out or \"what is picked\", " +
	"use plan_actions instead and do not try to carry anything out: when the audit ends, the user reviews the whole plan at once, " +
	"ticks what they want, and omega runs exactly the steps you planned, with no model and no further questions.\n" +
	"Each action must explain itself to a person who did not watch the audit:\n" +
	"- title: the change, in a few words, imperative (\"Remove 4 Claude Code worktrees already on GitHub\").\n" +
	"- why: what you found, with the figures and where (\"~/Library/Caches/Homebrew holds 8.9 GB of downloads for versions no " +
	"longer installed\").\n" +
	"- what: what will happen if it is ticked, what the user notices afterwards, and whether it can be undone.\n" +
	"- evidence: the output that shows it is safe (one line).\n" +
	"- steps: the exact tool calls that do it, in order (run_step, run_command, deep_clean, omega_cmd, write_file, mac_change, " +
	"dev_install, dev_activate), each with the same input you would give the tool. One action per decision: never bundle unrelated " +
	"changes, and give each deep_scan finding the user may want to keep its own action.\n" +
	"- bytes: the space it frees, measured, when it frees space.\n" +
	"Mark recommended only what you verified and would do on this machine. Things only the user can do by hand (a setting behind a " +
	"password, a purchase) are not actions: put them in the summary and in pending. End the stage with a short summary of what you " +
	"found and how many actions you planned."

// auditModePrompt is the run mode of an audit stage.
const auditModePrompt = " ULTRA AUDIT: read-only and unattended; every change goes into the plan with plan_actions, which the user reviews at the end."

// planTools are the tools a planned step may call: the ones that change
// something. A read-only call has no place in a plan; the audit makes it.
var planTools = map[string]bool{
	"run_step": true, "run_command": true, "write_file": true, "deep_clean": true, "omega_cmd": true,
	"mac_change": true, "dev_install": true, "dev_activate": true,
}

func planToolNames() []string {
	var out []string
	for n := range planTools {
		if goos != "darwin" && macOnlyTool(n) {
			continue
		}
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func planActionsSpec() llm.ToolSpec {
	return llm.ToolSpec{Name: "plan_actions", Description: "omega ultra audit: add changes to the plan the user reviews when the audit " +
		"ends. Nothing runs now. Every action explains itself (title, why, what, evidence) and lists the exact tool calls that carry it " +
		"out; omega checks each step now and runs exactly those steps, in order, for the actions the user ticks. Returns which actions " +
		"were added and why any was refused, so you can fix it and call again.",
		Properties: map[string]any{
			"actions": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":       str("short stable id, e.g. brew-cache or wt-api-fix-login"),
						"category": str("group as the user thinks of it, e.g. Updates, Cleanup, Docker, AI agents, Security, Mac"),
						"title":    str("the change, a few words, imperative"),
						"why":      str("what the audit found, with figures and where"),
						"what":     str("what happens if it is ticked, what the user notices afterwards, and whether it can be undone"),
						"impact":   str("expected effect in a few words, e.g. '~8.9 GB', 'fixes brew doctor'"),
						"bytes":    map[string]any{"type": "integer", "description": "space freed, measured, in bytes (0 if none)"},
						"risk":     map[string]any{"type": "string", "enum": []string{"normal", "dangerous"}},
						"evidence": str("one line: the output that shows it is safe and what it frees"),
						"recommended": map[string]any{"type": "boolean", "description": "you verified it and would do it on this machine; " +
							"it starts ticked in the review"},
						"reversible": map[string]any{"type": "boolean", "description": "it can be undone (the Trash, a backup, omega mac undo)"},
						"steps": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"tool":  map[string]any{"type": "string", "enum": planToolNames()},
									"input": map[string]any{"type": "object", "description": "the tool's input, as you would call it"},
								},
								"required": []string{"tool", "input"},
							},
						},
					},
					"required": []string{"id", "title", "why", "what", "risk", "evidence", "steps"},
				},
			},
		}, Required: []string{"actions"}}
}

// auditSpecs are the tools of an audit stage: propose_actions becomes
// plan_actions, and nothing that only removes is offered.
func auditSpecs(specs []llm.ToolSpec) []llm.ToolSpec {
	var out []llm.ToolSpec
	for _, s := range specs {
		switch s.Name {
		case "propose_actions":
			out = append(out, planActionsSpec())
		case "deep_clean":
		default:
			out = append(out, s)
		}
	}
	return out
}

func init() { handlers["plan_actions"] = toolPlanActions }

func toolPlanActions(_ context.Context, a *Agent, in map[string]any) (string, error) {
	if a.opts.Audit == nil {
		return "", fmt.Errorf("plan_actions exists only in omega ultra's audit; use propose_actions")
	}
	raw, _ := in["actions"].([]any)
	if len(raw) == 0 {
		return "", fmt.Errorf("actions is empty")
	}
	var added, refused []string
	for _, r := range raw {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		act, err := a.planAction(m)
		if err != nil {
			refused = append(refused, fmt.Sprintf("%s: %v", firstNonEmpty(getString(m, "id"), "(no id)"), err))
			continue
		}
		a.opts.Audit.add(act)
		added = append(added, act.ID)
		a.writeLog(Event{Kind: EvAssistant, Text: fmt.Sprintf("[planned %s: %s · %s · steps: %s]", act.ID, act.Title, act.Risk, showSteps(act.Steps))})
	}
	n, _ := a.opts.Audit.Count()
	a.emit(Event{Kind: EvToolEnd, Tool: "plan_actions", Command: "+" + strings.Join(added, " +"), Failed: len(added) == 0})
	var b strings.Builder
	if len(added) > 0 {
		fmt.Fprintf(&b, "Added to the plan (nothing was run): %s. The plan now has %d actions.\n", strings.Join(added, ", "), n)
	}
	if len(refused) > 0 {
		b.WriteString("Refused, fix and call plan_actions again with only these:\n- " + strings.Join(refused, "\n- ") + "\n")
	}
	b.WriteString("Go on with the audit; do not carry anything out.")
	if len(added) == 0 {
		return "", explained(errors.New(b.String()))
	}
	return b.String(), nil
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func showSteps(steps []PlanStep) string {
	var parts []string
	for _, s := range steps {
		parts = append(parts, s.Show)
	}
	return strings.Join(parts, " ; ")
}

// planAction reads one action of plan_actions and checks every step as the
// tool would check it, without running it.
func (a *Agent) planAction(m map[string]any) (PlannedAction, error) {
	act := PlannedAction{
		ID: cleanActionID(getString(m, "id")), Stage: a.opts.Task, Category: getString(m, "category"),
		Title: getString(m, "title"), Why: getString(m, "why"), What: getString(m, "what"), Impact: getString(m, "impact"),
		Risk: getString(m, "risk"), Evidence: getString(m, "evidence"),
	}
	act.Recommended, _ = m["recommended"].(bool)
	act.Reversible, _ = m["reversible"].(bool)
	if v, ok := m["bytes"].(float64); ok && v > 0 {
		act.Bytes = int64(v)
	}
	switch {
	case act.ID == "":
		return act, fmt.Errorf("id is required")
	case act.Title == "":
		return act, fmt.Errorf("title is required")
	case len(act.Why) < 12:
		return act, fmt.Errorf("why must say what the audit found, with figures")
	case len(act.What) < 12:
		return act, fmt.Errorf("what must say what happens if the user ticks it and whether it can be undone")
	case len(act.Evidence) < 12:
		return act, fmt.Errorf("evidence must say what verified it (the output that shows it is safe)")
	}
	if act.Risk != "dangerous" {
		act.Risk = "normal"
	}
	raw, _ := m["steps"].([]any)
	if len(raw) == 0 {
		return act, fmt.Errorf("steps is empty: list the tool calls that carry it out")
	}
	var found int64
	for i, r := range raw {
		sm, ok := r.(map[string]any)
		if !ok {
			return act, fmt.Errorf("step %d is not an object", i+1)
		}
		tool := getString(sm, "tool")
		input, _ := sm["input"].(map[string]any)
		if input == nil {
			input = map[string]any{}
		}
		show, risk, finds, err := a.checkStep(tool, input)
		if err != nil {
			return act, fmt.Errorf("step %d (%s): %v", i+1, tool, err)
		}
		if risk == safety.Dangerous {
			act.Risk = "dangerous"
		}
		for _, f := range finds {
			found += f.Size
			act.Findings = append(act.Findings, f)
		}
		act.Steps = append(act.Steps, PlanStep{Tool: tool, Input: input, Show: show})
	}
	if act.Bytes == 0 && found > 0 {
		act.Bytes = found
	}
	if act.Impact == "" && act.Bytes > 0 {
		act.Impact = "~" + humanBytes(act.Bytes)
	}
	return act, nil
}

// checkStep checks one step without running it: the tool exists for a
// plan, its input is complete, and what it runs is not blocked. It returns
// the step as the person reads it, how risky it is, and the deep_scan
// findings it removes.
func (a *Agent) checkStep(tool string, in map[string]any) (string, safety.Risk, []deepclean.Finding, error) {
	if !planTools[tool] {
		return "", 0, nil, fmt.Errorf("not a tool that changes something; one of %s", strings.Join(planToolNames(), ", "))
	}
	if goos != "darwin" && macOnlyTool(tool) {
		return "", 0, nil, fmt.Errorf("only exists on macOS")
	}
	switch tool {
	case "run_step":
		id := getString(in, "id")
		s, ok := steps.Find(id)
		if !ok {
			return "", 0, nil, fmt.Errorf("no step %q; use list_steps", id)
		}
		if !a.opts.SkipDetect && !s.Available() {
			return "", 0, nil, fmt.Errorf("step %q is not available on this machine", id)
		}
		if safety.Classify(s.Command) == safety.Blocked {
			return "", 0, nil, fmt.Errorf("step %q is blocked by omega's safety policy", id)
		}
		// A catalogue step is omega's own, reviewed with its risk: it is
		// high risk when it deletes what may be wanted, not because it
		// changes something.
		if s.AlwaysConfirm || safety.AlwaysConfirm(s.Command) {
			return s.Command, safety.Dangerous, nil, nil
		}
		return s.Command, safety.Normal, nil, nil
	case "run_command":
		cmd := getString(in, "command")
		if cmd == "" {
			return "", 0, nil, fmt.Errorf("command is required")
		}
		if getString(in, "reason") == "" {
			in["reason"] = "omega ultra plan"
		}
		risk := safety.Classify(cmd)
		if risk == safety.Blocked {
			return "", 0, nil, fmt.Errorf("blocked by omega's safety policy; it can never run")
		}
		if safety.AlwaysConfirm(cmd) {
			risk = safety.Dangerous
		}
		return cmd, risk, nil, nil
	case "write_file":
		path := getString(in, "path")
		if path == "" || getString(in, "reason") == "" {
			return "", 0, nil, fmt.Errorf("path and reason are required")
		}
		if _, ok := in["content"].(string); !ok {
			return "", 0, nil, fmt.Errorf("content is required")
		}
		return "write " + path + " (the original is kept as .omega-bak)", safety.Dangerous, nil, nil
	case "deep_clean":
		ids := getStrings(in, "ids")
		if len(ids) == 0 {
			return "", 0, nil, fmt.Errorf("ids is empty")
		}
		var finds []deepclean.Finding
		var shows []string
		risk := safety.Normal
		for _, id := range ids {
			f, ok := a.deepFinding(id)
			if !ok {
				return "", 0, nil, fmt.Errorf("no finding %q: run deep_scan first and use its ids", id)
			}
			if f.Verdict == deepclean.Keep {
				return "", 0, nil, fmt.Errorf("%s is kept and never removed: %s", f.Title, strings.Join(f.Reasons, "; "))
			}
			if f.Verdict != deepclean.Safe {
				// Something would be lost or rescued first.
				risk = safety.Dangerous
			}
			finds = append(finds, f)
			shows = append(shows, f.Action+" ("+humanBytes(f.Size)+")")
		}
		return strings.Join(shows, " ; "), risk, finds, nil
	case "omega_cmd":
		name := getString(in, "action")
		act, ok := ultraActions[name]
		if !ok {
			return "", 0, nil, fmt.Errorf("unknown action %q", name)
		}
		if !act.changes {
			return "", 0, nil, fmt.Errorf("%s only reads: run it now, during the audit", name)
		}
		args, err := act.args(in)
		if err != nil {
			return "", 0, nil, err
		}
		risk := safety.Normal
		if ultraDeletes[name] {
			risk = safety.Dangerous
		}
		return "omega " + strings.Join(args, " "), risk, nil, nil
	case "mac_change":
		id := getString(in, "change")
		if id == "" {
			return "", 0, nil, fmt.Errorf("change is required")
		}
		if getString(in, "reason") == "" {
			in["reason"] = "omega ultra plan"
		}
		risk := safety.Normal
		for _, p := range []string{"app-remove:", "perm-reset-all:", "disable:"} {
			if strings.HasPrefix(id, p) {
				risk = safety.Dangerous
			}
		}
		return "mac: " + id, risk, nil, nil
	case "dev_install", "dev_activate":
		tools := getStrings(in, "tools")
		if len(tools) == 0 {
			return "", 0, nil, fmt.Errorf("tools is empty")
		}
		verb := "install"
		if tool == "dev_activate" {
			verb = "activate in the shell"
		}
		return verb + ": " + strings.Join(tools, ", "), safety.Normal, nil, nil
	}
	return "", 0, nil, fmt.Errorf("unknown tool")
}

// ultraDeletes are the omega_cmd actions that delete files or end work in
// progress: high risk in the plan.
var ultraDeletes = map[string]bool{
	"artifacts_delete": true, "limits_enforce": true, "git_clean": true, "privacy_clean": true, "agents_idle": true,
}

// cleanActionID makes an id of lower-case words joined by dashes.
func cleanActionID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteRune('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

// ---------- carrying out the plan ----------

// NewExecutor makes an agent that carries out planned actions without a
// model: each step runs through the same tool as in a session, with the
// same checks, and the person's tick is the approval.
func NewExecutor(opts Options) *Agent {
	real := opts.Exec == nil
	if real {
		opts.Exec = runner.Run
	}
	if opts.Config == nil {
		opts.Config = config.Default()
	}
	opts.Ultra, opts.Preapproved, opts.Interactive, opts.Unattended = true, true, false, false
	opts.ReadOnly, opts.AutoYes, opts.Audit = false, false, nil
	a := &Agent{
		opts:    opts,
		model:   "-",
		events:  make(chan Event, 512),
		input:   make(chan string),
		started: time.Now(),
		real:    real,
		cache:   NewResultCache(),
	}
	a.openLog("omega ultra")
	return a
}

// RunPlanned carries out the actions, one after another, each one's steps
// in order; a step that fails ends its action and the next one starts. It
// reports how each action ended on done, and closes Events when it ends.
func (a *Agent) RunPlanned(ctx context.Context, acts []PlannedAction, done func(id, status, result string)) {
	defer close(a.events)
	defer a.closeLog()
	a.ctx = ctx
	if stop := a.startSudo(); stop != nil {
		defer stop()
	}
	var findings []deepclean.Finding
	for _, act := range acts {
		findings = append(findings, act.Findings...)
	}
	a.mu.Lock()
	a.deep = &deepclean.Report{Findings: findings}
	a.mu.Unlock()
	failed := 0
	for i, act := range acts {
		if ctx.Err() != nil {
			break
		}
		a.mu.Lock()
		a.conclusion = act.Title
		a.mu.Unlock()
		a.emit(Event{Kind: EvAssistant, Text: fmt.Sprintf("**%d/%d · %s** — %s", i+1, len(acts), act.Title, act.What)})
		status, result := ActionDone, ""
		for n, s := range act.Steps {
			if ctx.Err() != nil {
				status, result = ActionFailed, "stopped"
				break
			}
			b, _ := json.Marshal(s.Input)
			res := a.execute(ctx, llm.ToolCall{ID: fmt.Sprintf("plan_%s_%d", act.ID, n+1), Name: s.Tool, Input: b})
			if res.IsError || stepFailed(res.Content) {
				status, result = ActionFailed, firstLine(res.Content)
				break
			}
			result = lastLine(res.Content)
		}
		if status == ActionFailed {
			failed++
		}
		a.writeLog(Event{Kind: EvAssistant, Text: fmt.Sprintf("[plan action %s: %s · %s]", act.ID, status, result)})
		if done != nil {
			done(act.ID, status, result)
		}
	}
	text := i18n.N("plan.done_tally", len(acts), len(acts)-failed, len(acts))
	if ctx.Err() != nil {
		a.setExit(ExitCancelled)
	} else if failed > 0 {
		a.setExit(ExitError)
	}
	a.emit(Event{Kind: EvDone, Text: text, Stats: a.stats()})
}

// stepFailed reads a shell tool's answer: it reports the exit code first.
func stepFailed(out string) bool {
	if !strings.HasPrefix(out, "exit_code: ") {
		return false
	}
	return !strings.HasPrefix(out, "exit_code: 0\n") && strings.TrimSpace(out) != "exit_code: 0"
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
