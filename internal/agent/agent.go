// Package agent runs the model-driven maintenance loop: it sends the task to
// the model, executes the tools it asks for under the approval policy, and
// reports everything to the UI as Events.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lcajigasm/omega/internal/deepclean"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/macos"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/safety"
	"github.com/lcajigasm/omega/internal/sudo"
)

const maxTurns = 80

// maxCallsPerTurn is how many tool calls one reply of the model may run.
const maxCallsPerTurn = 16

// Process status values. ExitCancelled is the conventional 128+SIGINT.
const (
	ExitOK        = 0
	ExitError     = 1
	ExitCancelled = 130
)

// ExecFunc runs a shell command. It is runner.Run outside of tests and demos.
type ExecFunc func(ctx context.Context, command string, timeout time.Duration, onLine func(string)) runner.Result

// Options configure a run.
type Options struct {
	Provider string
	Model    string
	Escalate string // stronger model to switch to when a step fails; "" disables
	Approval string // config.Approve*
	AutoYes  bool   // --yes: approve everything except blocked and always-confirm actions
	ReadOnly bool   // only Safe actions are allowed until the user picks actions
	// DryRun is --dry-run: read-only for the whole session. Unlike the
	// analyze task, a pick on a card does not lift it.
	DryRun      bool
	Interactive bool // wait for follow-up input after the model finishes
	// OneShot ends the session when the model finishes its turn, although
	// someone is there to answer cards and passwords: a stage of the guided
	// run goes on to the next one instead of waiting for a message.
	OneShot bool
	// Unattended is a run nobody watches (omega full --unattended): every
	// option offered with propose_actions is taken, and the actions that
	// always need a person are skipped instead of waiting for one.
	Unattended bool
	Timeout    time.Duration
	Config     *config.Config
	Exec       ExecFunc
	SkipDetect bool // run catalogue steps without checking they are installed (demo mode)
	// SudoPassword, when set (omega --sudo), answers sudo prompts without asking.
	SudoPassword string
	// Doctor runs omega's preflight check and returns its report. The CLI
	// sets it; the check lives there because it knows the providers and keys.
	Doctor func() string
	// Models returns the provider's models with score and price and the two
	// that automatic mode uses, as `omega models` prints them.
	Models func() string
	// MaxCost limits the estimated session cost in USD.
	MaxCost float64
	// MaxTurns limits the agent turns (defaults to 80).
	MaxTurns int
	// Version, Task and Command go into the transcript's header, so a run
	// can be told apart from another later: which omega, which task and the
	// exact command line (omega full --unattended, a stage, a question).
	Version string
	Task    string
	Command string
	// Cache, when set, is shared with the other stages of the same run:
	// what one stage measured read-only is not measured again by the next.
	Cache *ResultCache
	// Ultra is a stage of omega ultra: the deep tools are offered, every
	// option that changes something must carry its evidence, and every
	// change is put to the user.
	Ultra bool
}

// Agent drives one maintenance session.
type Agent struct {
	opts Options
	chat llm.Chat
	// turnCancel stops the running turn (CancelTurn); nil between turns.
	turnCancel context.CancelFunc
	events     chan Event
	input      chan string
	log        *os.File
	usage      llm.Usage
	cost       float64
	started    time.Time
	real       bool // commands run on this machine (not a test or demo)
	ctx        context.Context

	// Changed by the UI, or by a tool that fails, while the run goes on.
	mu       sync.Mutex
	mode     Mode
	exitCode int
	model    string
	// The automatic switch to the stronger model: escalated while the run
	// is on it, calm counts the turns since without a failure, picked is
	// set once the user chose a model (no automatic switch after that) and
	// noSwitch once the provider refused one.
	escalated bool
	calm      int
	toolFails int // tool calls that ended in a failure
	picked    bool
	noSwitch  bool
	costKnown bool
	// guessCost is what turns of a model with no known price would cost at
	// llm.CautiousPrice; it only counts toward --max-cost.
	guessCost  float64
	warnedCost bool
	nextModel  string   // model the user picked, applied before the next turn
	notes      []string // messages for the model about what the user changed

	// What the web brought into this session. webSeen are the addresses
	// web_search returned and userHosts the sites the user named: fetch_url
	// reads those without asking. tainted is set once any page or search
	// result has been read: from then on a note for future runs needs a yes.
	webSeen   map[string]bool
	userHosts map[string]bool
	tainted   bool

	macInv *macos.Inventory // read once per run; dropped after a change

	accessMu    sync.Mutex      // one permission card at a time
	accessAsked map[string]bool // privacy permissions already asked for
	logMu       sync.Mutex      // serializes transcript writes from parallel tools
	calls       map[string]*callLog
	batching    int                // > 0 while a batch of calls runs in parallel (under logMu)
	costBy      map[string]float64 // cost of each model used, for the transcript
	ok          int                // calls that succeeded (under logMu)
	failed      []string           // calls that failed (under logMu)

	// What run_command was told by a tool that wants a confirmation of its
	// own (mo purge: "use --yes to confirm"): the program, so that adding
	// --yes or --force to it afterwards is asked about every time.
	refusedConfirm map[string]bool
	cache          *ResultCache // read-only results kept for the session (or the run)
	// deep is what the last deep_scan found (omega ultra).
	deep       *deepclean.Report
	conclusion string // the last thing the model wrote: what the session concluded
	lastCall   uint64 // numbers the shell calls, for the transcript (under mu)
}

// nextCall numbers a tool call: two identical commands of one batch run
// at the same time and must not share their place in the transcript.
func (a *Agent) nextCall() uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastCall++
	return a.lastCall
}

// New creates an agent for the given provider.
func New(p llm.Provider, opts Options) *Agent {
	real := opts.Exec == nil
	if real {
		opts.Exec = runner.Run
	}
	if opts.Provider == "" {
		opts.Provider = p.Name()
	}
	notes, _ := os.ReadFile(config.NotesPath())
	sys := systemPrompt(promptNotes(string(notes), maxPromptNotes), opts.Config.SkipSteps, opts.ReadOnly) + runModePrompt(opts) + pendingPrompt()
	specs := toolSpecs()
	if opts.Unattended {
		specs = upkeepTools(specs)
	}
	if opts.Ultra && !opts.Unattended {
		specs = append(specs, ultraToolSpecs()...)
	}
	a := &Agent{
		opts:    opts,
		model:   opts.Model,
		chat:    p.NewChat(opts.Model, sys, specs),
		events:  make(chan Event, 512),
		input:   make(chan string),
		started: time.Now(),
		real:    real,
		cache:   opts.Cache,
	}
	if a.cache == nil {
		a.cache = NewResultCache()
	}
	switch {
	case opts.ReadOnly:
		a.mode = ModeReadOnly
	case opts.AutoYes:
		a.mode = ModeAuto
	}
	_, a.costKnown = llm.PriceFor(opts.Provider, opts.Model)
	a.openLog(p.Name())
	return a
}

// Conclusion is the last thing the model wrote in this session: in a stage
// of a guided run, its summary of what was done and what is left.
func (a *Agent) Conclusion() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.conclusion
}

// Events is the stream the UI consumes. It is closed when the run ends.
func (a *Agent) Events() <-chan Event { return a.events }

// Input sends a follow-up message from the user while the agent is idle.
// An empty string ends the session.
func (a *Agent) Input() chan<- string { return a.input }

// LogPath returns where this run's transcript is written.
func (a *Agent) LogPath() string {
	if a.log == nil {
		return ""
	}
	return a.log.Name()
}

func (a *Agent) emit(e Event) {
	// What the interface shows of a tool call is written with printable
	// characters only: a path or a reason with an escape sequence or a
	// mark that reorders text would read as something else on the card and
	// in the transcript.
	e.Command, e.Reason = visible(e.Command), visible(e.Reason)
	if e.Kind == EvToolEnd && (e.Failed || e.ExitCode != 0) {
		a.mu.Lock()
		a.toolFails++
		a.mu.Unlock()
	}
	a.writeLog(e)
	a.events <- e
}

func (a *Agent) stats() Stats {
	a.mu.Lock()
	model, known := a.model, a.costKnown
	a.mu.Unlock()
	return Stats{Usage: a.usage, Cost: a.cost, CostKnown: known, Model: model, Elapsed: time.Since(a.started)}
}

// currentModel is the model of the next request. A failed step changes it
// from another goroutine, so it is read under the lock.
func (a *Agent) currentModel() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.model
}

// compact drops the old tool outputs from the conversation and returns how
// many it dropped.
func (a *Agent) compact() int {
	c, ok := a.chat.(llm.Compactor)
	if !ok {
		return 0
	}
	n := c.Compact()
	if n > 0 {
		a.writeLog(Event{Kind: EvAssistant, Text: fmt.Sprintf("[history compacted: %d old tool outputs omitted]", n)})
	}
	return n
}

// ExitCode is the status of a finished run: 0, ExitError or ExitCancelled.
// It is safe to call after Events has been closed.
func (a *Agent) ExitCode() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.exitCode
}

func (a *Agent) setExit(code int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if code == ExitCancelled || a.exitCode == 0 {
		a.exitCode = code
	}
}

func (a *Agent) clearExit() {
	a.mu.Lock()
	a.exitCode = 0
	a.mu.Unlock()
}

// Run executes the task until the model stops and, in interactive mode, the
// user ends the session.
func (a *Agent) Run(ctx context.Context, task string) {
	defer close(a.events)
	defer a.closeLog()
	a.ctx = ctx
	a.writeLog(Event{Kind: EvAssistant, Text: "> " + task})
	if a.real {
		if b, err := sudo.Start(a.askPassword, a.opts.SudoPassword); err == nil {
			// OMEGA_SUDO tells a step whether this run can answer sudo:
			// yes with --sudo, ask when someone can type the password,
			// no when nobody can (the brew step then leaves out the casks
			// that need it instead of failing on them).
			sudoMode := "no"
			switch {
			case a.opts.SudoPassword != "":
				sudoMode = "yes"
			case a.opts.Interactive && !a.opts.Unattended:
				sudoMode = "ask"
			}
			runner.Configure(append(b.Env(), "OMEGA_SUDO="+sudoMode), b.PathPrefix())
			defer func() {
				runner.Configure(nil, "")
				b.Close()
			}()
		}
	}

	text := task
	var results []llm.ToolResult
	compacted := false // the history was already compacted for the message being sent
	for turn := 0; ; turn++ {
		if ctx.Err() != nil {
			a.setExit(ExitCancelled)
			a.emit(Event{Kind: EvDone, Text: i18n.T("agent.cancelled"), Stats: a.stats()})
			return
		}
		limit := maxTurns
		if a.opts.MaxTurns > 0 {
			limit = a.opts.MaxTurns
		}
		if turn >= limit {
			a.setExit(ExitError)
			a.emit(Event{Kind: EvError, Text: i18n.T("agent.turn_limit", limit)})
			// The results of the last tools stay: they go with the next
			// message. Dropping them leaves the model's tool calls without
			// an answer, and the provider then refuses every message.
			text = ""
			if !a.waitInput(ctx, &text) {
				return
			}
			a.clearExit()
			turn = -1 // the loop adds one: a full allowance again
			continue
		}

		// The task's own text or what the user typed, never a tool result.
		a.noteUserText(text)
		text = a.applyChanges(text)
		a.emit(Event{Kind: EvThinking, Stats: a.stats()})
		tctx, tcancel := a.startTurn(ctx)
		reply, err := a.chat.Send(tctx, text, results)
		if err != nil {
			stopped := tctx.Err() != nil
			a.endTurn(tcancel)
			if ctx.Err() != nil {
				a.setExit(ExitCancelled)
				a.emit(Event{Kind: EvDone, Text: i18n.T("agent.cancelled"), Stats: a.stats()})
				return
			}
			if stopped {
				// The user stopped this turn, not the session: the message
				// was rolled back, and the next one goes with what they type.
				a.writeLog(Event{Kind: EvError, Text: "turn cancelled by the user"})
				if !a.waitInput(ctx, &text) {
					return
				}
				continue
			}
			// The conversation no longer fits in the model's window: drop
			// the old tool outputs and send the same message again, once.
			if !compacted && llm.IsContextLength(err) && a.compact() > 0 {
				compacted = true
				continue
			}
			a.setExit(ExitError)
			a.emit(Event{Kind: EvError, Text: i18n.T("agent.model_error", err.Error())})
			// The failed message was rolled back; let the user retry or quit.
			if !a.opts.Interactive || a.opts.OneShot {
				a.emit(Event{Kind: EvDone, Stats: a.stats()})
				return
			}
			if !a.waitInput(ctx, &text) {
				return
			}
			a.clearExit()
			if text == "" {
				text = "continue"
			}
			continue
		}
		a.clearExit()
		compacted = false
		if reply.Truncated {
			// The model was cut before it finished: a run that ends here
			// did not do its task.
			a.setExit(ExitError)
		}
		a.usage = a.usage.Add(reply.Usage)
		model := a.currentModel()
		if price, ok := llm.PriceFor(a.opts.Provider, model); ok {
			c := price.Cost(reply.Usage)
			a.cost += c
			a.mu.Lock()
			if a.costBy == nil {
				a.costBy = map[string]float64{}
			}
			a.costBy[model] += c
			a.mu.Unlock()
		} else {
			a.mu.Lock()
			a.costKnown = false
			a.mu.Unlock()
			// The real price is unknown, but a cost limit must still
			// hold: it counts at a cautious price, and says so once.
			if a.opts.MaxCost > 0 {
				a.guessCost += llm.CautiousPrice.Cost(reply.Usage)
				if !a.warnedCost {
					a.warnedCost = true
					a.emit(Event{Kind: EvError, Text: i18n.T("agent.cost_unknown", model, a.opts.MaxCost)})
				}
			}
		}
		a.emit(Event{Kind: EvUsage, Stats: a.stats()})
		if a.opts.MaxCost > 0 && a.cost+a.guessCost >= a.opts.MaxCost {
			// The run was cut short: whatever was left to do was not done.
			a.setExit(ExitError)
			a.emit(Event{Kind: EvDone, Text: i18n.T("agent.cost_limit", a.opts.MaxCost), Stats: a.stats()})
			return
		}
		// Every turn resends the whole history. Before it outgrows the
		// model's window, the old tool outputs go.
		if reply.Usage.Input() >= llm.CompactAt(a.opts.Provider, model) {
			a.compact()
		}
		if t := strings.TrimSpace(reply.Text); t != "" {
			a.mu.Lock()
			a.conclusion = t
			a.mu.Unlock()
			a.emit(Event{Kind: EvAssistant, Text: t})
		}

		if len(reply.Calls) == 0 {
			a.endTurn(tcancel)
			results, text = nil, ""
			if !a.waitInput(ctx, &text) {
				return
			}
			continue
		}

		text = ""
		before := a.failures()
		results = a.executeAll(tctx, reply.Calls)
		cancelled := tctx.Err() != nil && ctx.Err() == nil
		a.endTurn(tcancel)
		a.settle(a.failures() > before)
		if cancelled {
			// What ran was stopped by the user: the results say so, and
			// they go to the model with the user's next words instead of
			// starting another turn on its own.
			a.writeLog(Event{Kind: EvError, Text: "turn cancelled by the user"})
			if !a.waitInput(ctx, &text) {
				return
			}
		}
	}
}

// startTurn gives one turn its own context, so CancelTurn can stop it
// without ending the session.
func (a *Agent) startTurn(ctx context.Context) (context.Context, context.CancelFunc) {
	tctx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.turnCancel = cancel
	a.mu.Unlock()
	return tctx, cancel
}

func (a *Agent) endTurn(cancel context.CancelFunc) {
	a.mu.Lock()
	a.turnCancel = nil
	a.mu.Unlock()
	cancel()
}

// CancelTurn stops the model's reply or the commands of the current turn
// and leaves the session waiting for the user, as ctrl+c in a shell stops
// the command and keeps the shell. It reports whether a turn was running.
// A turn is stopped once: a second ctrl+c before it has wound down finds
// nothing to stop, so the caller ends the session as the user meant.
func (a *Agent) CancelTurn() bool {
	a.mu.Lock()
	cancel := a.turnCancel
	a.turnCancel = nil
	a.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

// calmTurns is how many turns in a row without a failed call bring the run
// back from the stronger model to the one it started with.
const calmTurns = 4

// failures counts the calls that failed so far.
func (a *Agent) failures() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.toolFails
}

// settle returns to the starting model once the run has gone calmTurns
// turns on the stronger one without a failure: what it was needed for is
// done, and routine work goes back to the cheaper model.
func (a *Agent) settle(failed bool) {
	a.mu.Lock()
	if !a.escalated || a.picked {
		a.mu.Unlock()
		return
	}
	if failed {
		a.calm = 0
		a.mu.Unlock()
		return
	}
	a.calm++
	back := a.calm >= calmTurns && a.opts.Model != "" && a.model != a.opts.Model
	a.mu.Unlock()
	if !back {
		return
	}
	if a.switchModel(a.opts.Model, fmt.Sprintf("%d turns without a failure", calmTurns)) {
		a.mu.Lock()
		a.escalated, a.calm = false, 0
		a.mu.Unlock()
		a.emit(Event{Kind: EvModel, Text: a.opts.Model, Stats: a.stats()})
	}
}

// executeAll runs every call. A batch that is entirely read-only runs at
// once: several checks then finish in the time of the slowest one. A call
// that changes the machine or may ask the user keeps the whole batch in
// order, so a question is never racing another command.
func (a *Agent) executeAll(ctx context.Context, calls []llm.ToolCall) []llm.ToolResult {
	results := make([]llm.ToolResult, len(calls))
	// A turn has a ceiling: a reply with hundreds of calls (a page that
	// talked the model into it, or a loop) would otherwise run them all
	// before anyone could look. The rest are answered, not run.
	if len(calls) > maxCallsPerTurn {
		for i := maxCallsPerTurn; i < len(calls); i++ {
			results[i] = llm.ToolResult{CallID: calls[i].ID, IsError: true,
				Content: fmt.Sprintf("not run: at most %d tool calls run per turn; ask for this one again in the next turn if it is still needed", maxCallsPerTurn)}
		}
		a.writeLog(Event{Kind: EvError, Text: fmt.Sprintf("the model asked for %d tool calls in one turn; only the first %d ran", len(calls), maxCallsPerTurn)})
		done := a.executeAll(ctx, calls[:maxCallsPerTurn])
		copy(results, done)
		return results
	}
	if len(calls) < 2 || !allReadOnly(calls) {
		for i, call := range calls {
			results[i] = a.execute(ctx, call)
		}
		return results
	}
	// Their output goes to the transcript one call at a time, each under its
	// own header, instead of line by line as it arrives.
	a.logMu.Lock()
	a.batching++
	a.logMu.Unlock()
	defer func() {
		a.logMu.Lock()
		a.batching--
		a.logMu.Unlock()
	}()
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(i int, call llm.ToolCall) {
			defer wg.Done()
			results[i] = a.execute(ctx, call)
		}(i, call)
	}
	wg.Wait()
	return results
}

// waitInput blocks for a follow-up in interactive mode. It returns false when
// the session should end.
func (a *Agent) waitInput(ctx context.Context, text *string) bool {
	if !a.opts.Interactive || a.opts.OneShot {
		a.emit(Event{Kind: EvDone, Stats: a.stats()})
		return false
	}
	a.emit(Event{Kind: EvIdle, Stats: a.stats()})
	select {
	case <-ctx.Done():
		a.setExit(ExitCancelled)
		a.emit(Event{Kind: EvDone, Text: i18n.T("agent.cancelled"), Stats: a.stats()})
		return false
	case msg, ok := <-a.input:
		if !ok || strings.TrimSpace(msg) == "" {
			a.emit(Event{Kind: EvDone, Stats: a.stats()})
			return false
		}
		*text = msg
		a.writeLog(Event{Kind: EvAssistant, Text: "> " + msg})
		return true
	}
}

// markTainted records that text from the web is now in the conversation.
func (a *Agent) markTainted() {
	a.mu.Lock()
	a.tainted = true
	a.mu.Unlock()
}

func (a *Agent) isTainted() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.tainted
}

// Mode returns the current approval mode.
func (a *Agent) Mode() Mode {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.mode
}

// SetMode changes the approval mode. It is safe to call from the UI while
// the agent runs; the model is told when read-only mode starts or ends.
func (a *Agent) SetMode(m Mode) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if m == a.mode {
		return
	}
	switch {
	case m == ModeReadOnly:
		a.notes = append(a.notes, "The user switched to read-only mode: from now on only read-only commands and steps "+
			"are allowed. Offer any change with propose_actions instead of running it.")
	case a.mode == ModeReadOnly:
		a.notes = append(a.notes, "The user switched off read-only mode: you may now make changes, "+
			"subject to the approval rules.")
	}
	a.mode = m
}

// SetModel switches to another model from the next turn on. A model the user
// picks is kept: the automatic switch to the stronger model no longer applies.
func (a *Agent) SetModel(model string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.nextModel = model
}

// applyChanges applies what the user changed from the UI before the next
// request and returns text with the notes for the model appended.
func (a *Agent) applyChanges(text string) string {
	a.mu.Lock()
	next, notes := a.nextModel, a.notes
	a.nextModel, a.notes = "", nil
	a.mu.Unlock()
	if next != "" && next != a.currentModel() {
		a.mu.Lock()
		a.picked = true // no automatic switch after this
		a.mu.Unlock()
		a.switchModel(next, "picked by the user")
	}
	if len(notes) == 0 {
		return text
	}
	note := "[" + strings.Join(notes, " ") + "]"
	a.writeLog(Event{Kind: EvAssistant, Text: note})
	if text == "" {
		return note
	}
	return text + "\n\n" + note
}

// switchModel moves the conversation to another model. A provider can
// refuse (OpenCode Zen cannot carry a conversation from Claude to a model
// of its other endpoint): the run then goes on with the model it had, and
// says so.
func (a *Agent) switchModel(model, why string) bool {
	if err := llm.SwitchModel(a.chat, model); err != nil {
		a.mu.Lock()
		a.noSwitch = true // the provider cannot: no automatic switch after this
		a.mu.Unlock()
		a.emit(Event{Kind: EvError, Text: err.Error()})
		return false
	}
	_, known := llm.PriceFor(a.opts.Provider, model)
	a.mu.Lock()
	a.model = model
	if !known {
		a.costKnown = false
	}
	a.mu.Unlock()
	line := "[model " + model + "]"
	if why != "" {
		line = "[model " + model + ": " + why + "]"
	}
	a.writeLog(Event{Kind: EvAssistant, Text: line})
	return true
}

// escalate switches to the stronger model when something fails, so
// routine work stays on the cheap model; settle switches back after a few
// calm turns. A model the user picked wins.
func (a *Agent) escalate(why string) {
	a.mu.Lock()
	picked := a.picked || a.nextModel != ""
	model := a.opts.Escalate
	if picked || a.escalated || a.noSwitch || model == "" || model == a.model {
		if a.escalated {
			a.calm = 0
		}
		a.mu.Unlock()
		return
	}
	a.escalated, a.calm = true, 0
	a.mu.Unlock()
	if a.switchModel(model, why) {
		a.emit(Event{Kind: EvModel, Text: model, Stats: a.stats()})
		return
	}
	a.mu.Lock()
	a.escalated = false
	a.mu.Unlock()
}

// authorize applies the approval policy and asks the user when needed.
// always forces a question even under --yes or "approve all", for actions
// such as Docker cleanups that delete data the user may want to keep, and
// for writes to the files that decide what runs at login.
func (a *Agent) authorize(ctx context.Context, tool, command, reason string, risk safety.Risk, always bool) (bool, string) {
	deny := func(why string) (bool, string) {
		a.emit(Event{Kind: EvDenied, Tool: tool, Command: command, Reason: why})
		return false, why
	}
	if risk == safety.Blocked {
		return deny("Blocked by omega's safety policy; this command can never run.")
	}
	mode := a.Mode()
	if mode == ModeReadOnly && risk > safety.Safe {
		return deny("Read-only mode: this action would change the system and was rejected. " +
			"Offer it to the user with propose_actions instead.")
	}
	if a.opts.Unattended && risk > safety.Safe && tool == "run_command" {
		// A command of the model's own: the catalogue steps are omega's,
		// checked one by one; this one is read for what it would change.
		if what := safety.NotUpkeep(command); what != "" {
			return deny("Unattended run: this command changes " + what + ", which is how the user set up this machine, not upkeep, " +
				"so it was not run. Do not retry it or an equivalent; put it in your summary as a suggestion for the user. " +
				"Updates and cleanups go through run_step, whose steps leave out what needs a person.")
		}
	}
	if a.opts.Unattended && risk > safety.Safe {
		if what := safety.PersonalSetup(command); what != "" {
			return deny("Unattended run: " + what + " is how the user arranged this machine, not maintenance, so it was left as it is. " +
				"Do not retry it or an equivalent; mention it in your summary as a suggestion for the user.")
		}
	}
	if always && a.opts.Unattended {
		return deny("Unattended run: this action always needs a person (it deletes data the user may want to keep, or changes " +
			"what the machine runs at login), so it was skipped. Do not retry it; list it in your summary as something to do by hand.")
	}
	if !always {
		if risk == safety.Safe {
			return true, ""
		}
		if mode == ModeAuto || a.opts.Approval == config.ApproveNever {
			a.logApproval(tool, command, "approved without asking (automatic mode or approval = never)")
			return true, ""
		}
		if risk == safety.Normal && a.opts.Approval != config.ApproveAll {
			a.logApproval(tool, command, "approved without asking (routine change under the approval policy)")
			return true, ""
		}
	}
	reply := make(chan Decision, 1)
	a.emit(Event{Kind: EvConfirm, Tool: tool, Command: command, Reason: reason, Risk: risk, Always: always, Reply: reply})
	select {
	case <-ctx.Done():
		return false, "cancelled"
	case d := <-reply:
		switch d {
		case ApproveAll:
			if !always {
				a.SetMode(ModeAuto)
			}
			a.logApproval(tool, command, "approved by the user (and everything after it)")
			return true, ""
		case Approve:
			a.logApproval(tool, command, "approved by the user")
			return true, ""
		}
	}
	return deny("The user denied this action. Do not retry it or an equivalent; carry on with the rest.")
}

// logApproval writes who let a change through: the transcript then shows
// whether a deletion was the user's yes or the policy's.
func (a *Agent) logApproval(tool, command, how string) {
	a.writeLog(Event{Kind: EvAssistant, Text: fmt.Sprintf("[%s %s] %s", tool, how, visible(command))})
}

// askPassword asks the user for the sudo password in the interface. It runs
// on the sudo bridge's goroutine while a command waits for it.
func (a *Agent) askPassword(prompt string, retry bool) (string, bool) {
	if !a.opts.Interactive {
		return "", false
	}
	reply := make(chan string, 1)
	a.emit(Event{Kind: EvPassword, Text: prompt, Failed: retry, Secret: reply})
	select {
	case <-a.ctx.Done():
		return "", false
	case pw := <-reply:
		return pw, pw != ""
	}
}

// choose shows a selection card and returns the IDs the user picked.
func (a *Agent) choose(ctx context.Context, question string, opts []Choice, multi bool) []string {
	if !a.opts.Interactive {
		return nil
	}
	reply := make(chan []string, 1)
	a.emit(Event{Kind: EvChoice, Text: question, Choices: opts, Multi: multi, Picks: reply})
	select {
	case <-ctx.Done():
		return nil
	case ids := <-reply:
		return ids
	}
}

func (a *Agent) execute(ctx context.Context, call llm.ToolCall) llm.ToolResult {
	res := llm.ToolResult{CallID: call.ID}
	h, ok := handlers[call.Name]
	if !ok {
		res.Content, res.IsError = "unknown tool: "+call.Name, true
		return res
	}
	if what, off := notUpkeepTools[call.Name]; off && a.opts.Unattended {
		// Not offered in this mode; a model that calls it anyway is told so.
		res.Content, res.IsError = a.notUpkeep(call.Name, "", "the "+call.Name+" tool changes "+what).Error(), true
		return res
	}
	if goos != "darwin" && macOnlyTool(call.Name) {
		res.Content, res.IsError = call.Name+" only exists on macOS; this machine runs "+osName()+". Tell the user that part of omega is Mac-only.", true
		return res
	}
	var in map[string]any
	if err := json.Unmarshal(call.Input, &in); err != nil {
		res.Content, res.IsError = "invalid JSON arguments: "+err.Error(), true
		return res
	}
	out, err := h(ctx, a, in)
	if service, blocked := blockedAccess(out, err); blocked {
		out, err = a.regainAccess(ctx, call, service, func() (string, error) { return h(ctx, a, in) }, out, err)
	}
	if err != nil {
		res.Content, res.IsError = err.Error(), true
		// A denial or a finished card already carries the text. Anything else
		// would vanish: the model sees it, the screen would not.
		if !isExplained(err) {
			a.emit(Event{Kind: EvError, Tool: call.Name, Text: err.Error()})
		}
		return res
	}
	if loggedResult[call.Name] {
		a.logResult(call.Name, out)
	}
	res.Content = out
	return res
}

// explainedError marks a failure the UI already shows on a card.
type explainedError struct{ msg string }

func (e explainedError) Error() string { return e.msg }

func explained(err error) error {
	if err == nil {
		return nil
	}
	var already explainedError
	if errors.As(err, &already) {
		return err
	}
	return explainedError{msg: err.Error()}
}

func isExplained(err error) bool {
	var already explainedError
	return errors.As(err, &already)
}
