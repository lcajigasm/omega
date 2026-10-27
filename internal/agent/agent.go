// Package agent runs the model-driven maintenance loop: it sends the task to
// the model, executes the tools it asks for under the approval policy, and
// reports everything to the UI as Events.
package agent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/safety"
	"github.com/lcajigasm/omega/internal/sudo"
)

const maxTurns = 80

// ExecFunc runs a shell command. It is runner.Run outside of tests and demos.
type ExecFunc func(ctx context.Context, command string, timeout time.Duration, onLine func(string)) runner.Result

// Options configure a run.
type Options struct {
	Provider    string
	Model       string
	Escalate    string // stronger model to switch to when a step fails; "" disables
	Approval    string // config.Approve*
	AutoYes     bool   // --yes: approve everything except blocked and always-confirm actions
	ReadOnly    bool   // only Safe actions are allowed until the user picks actions
	Interactive bool   // wait for follow-up input after the model finishes
	Timeout     time.Duration
	Config      *config.Config
	Exec        ExecFunc
	SkipDetect  bool // run catalogue steps without checking they are installed (demo mode)
	// SudoPassword, when set (omega --sudo), answers sudo prompts without asking.
	SudoPassword string
}

// Agent drives one maintenance session.
type Agent struct {
	opts       Options
	chat       llm.Chat
	model      string
	escalated  bool
	events     chan Event
	input      chan string
	approveAll bool
	log        *os.File
	usage      llm.Usage
	cost       float64
	costKnown  bool
	started    time.Time
	real       bool // commands run on this machine (not a test or demo)
	ctx        context.Context
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
	sys := systemPrompt(string(notes), opts.Config.SkipSteps, opts.ReadOnly)
	a := &Agent{
		opts:    opts,
		model:   opts.Model,
		chat:    p.NewChat(opts.Model, sys, toolSpecs()),
		events:  make(chan Event, 512),
		input:   make(chan string),
		started: time.Now(),
		real:    real,
	}
	_, a.costKnown = llm.PriceFor(opts.Provider, opts.Model)
	a.openLog(p.Name())
	return a
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
	a.writeLog(e)
	a.events <- e
}

func (a *Agent) stats() Stats {
	return Stats{Usage: a.usage, Cost: a.cost, CostKnown: a.costKnown, Model: a.model, Elapsed: time.Since(a.started)}
}

// Run executes the task until the model stops and, in interactive mode, the
// user ends the session.
func (a *Agent) Run(ctx context.Context, task string) {
	defer close(a.events)
	defer a.closeLog()
	a.ctx = ctx
	if a.real {
		if b, err := sudo.Start(a.askPassword, a.opts.SudoPassword); err == nil {
			runner.Configure(b.Env(), b.PathPrefix())
			defer func() {
				runner.Configure(nil, "")
				b.Close()
			}()
		}
	}

	text := task
	var results []llm.ToolResult
	for turn := 0; ; turn++ {
		if ctx.Err() != nil {
			a.emit(Event{Kind: EvDone, Text: i18n.T("agent.cancelled"), Stats: a.stats()})
			return
		}
		if turn >= maxTurns {
			a.emit(Event{Kind: EvError, Text: i18n.T("agent.turn_limit", maxTurns)})
			results, text = nil, ""
			if !a.waitInput(ctx, &text) {
				return
			}
			turn = 0
			continue
		}

		a.emit(Event{Kind: EvThinking, Stats: a.stats()})
		reply, err := a.chat.Send(ctx, text, results)
		if err != nil {
			if ctx.Err() != nil {
				a.emit(Event{Kind: EvDone, Text: i18n.T("agent.cancelled"), Stats: a.stats()})
				return
			}
			a.emit(Event{Kind: EvError, Text: i18n.T("agent.model_error", err.Error())})
			// The failed message was rolled back; let the user retry or quit.
			if !a.opts.Interactive {
				a.emit(Event{Kind: EvDone, Stats: a.stats()})
				return
			}
			if !a.waitInput(ctx, &text) {
				return
			}
			if text == "" {
				text = "continue"
			}
			continue
		}
		a.usage = a.usage.Add(reply.Usage)
		if price, ok := llm.PriceFor(a.opts.Provider, a.model); ok {
			a.cost += price.Cost(reply.Usage)
		} else {
			a.costKnown = false
		}
		a.emit(Event{Kind: EvUsage, Stats: a.stats()})
		if t := strings.TrimSpace(reply.Text); t != "" {
			a.emit(Event{Kind: EvAssistant, Text: t})
		}

		if len(reply.Calls) == 0 {
			results, text = nil, ""
			if !a.waitInput(ctx, &text) {
				return
			}
			continue
		}

		text = ""
		results = nil
		for _, call := range reply.Calls {
			results = append(results, a.execute(ctx, call))
		}
	}
}

// waitInput blocks for a follow-up in interactive mode. It returns false when
// the session should end.
func (a *Agent) waitInput(ctx context.Context, text *string) bool {
	if !a.opts.Interactive {
		a.emit(Event{Kind: EvDone, Stats: a.stats()})
		return false
	}
	a.emit(Event{Kind: EvIdle, Stats: a.stats()})
	select {
	case <-ctx.Done():
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

// escalate switches to the stronger model once, the first time something
// fails, so routine work stays on the cheap model.
func (a *Agent) escalate() {
	if a.escalated || a.opts.Escalate == "" || a.opts.Escalate == a.model {
		return
	}
	a.escalated = true
	a.model = a.opts.Escalate
	a.chat.SetModel(a.model)
	if _, ok := llm.PriceFor(a.opts.Provider, a.model); !ok {
		a.costKnown = false
	}
	a.emit(Event{Kind: EvModel, Text: a.model, Stats: a.stats()})
}

// authorize applies the approval policy and asks the user when needed.
// always forces a question even under --yes or "approve all", for actions
// such as Docker cleanups that delete data the user may want to keep.
func (a *Agent) authorize(ctx context.Context, tool, command, reason string, risk safety.Risk, always bool) (bool, string) {
	deny := func(why string) (bool, string) {
		a.emit(Event{Kind: EvDenied, Tool: tool, Command: command, Reason: why})
		return false, why
	}
	if risk == safety.Blocked {
		return deny("Blocked by omega's safety policy; this command can never run.")
	}
	if a.opts.ReadOnly && risk > safety.Safe {
		return deny("Read-only mode: this action would change the system and was rejected. " +
			"Offer it to the user with propose_actions instead.")
	}
	if !always {
		if risk == safety.Safe || a.opts.AutoYes || a.approveAll || a.opts.Approval == config.ApproveNever {
			return true, ""
		}
		if risk == safety.Normal && a.opts.Approval != config.ApproveAll {
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
				a.approveAll = true
			}
			return true, ""
		case Approve:
			return true, ""
		}
	}
	return deny("The user denied this action. Do not retry it or an equivalent; carry on with the rest.")
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
	var in map[string]any
	if err := json.Unmarshal(call.Input, &in); err != nil {
		res.Content, res.IsError = "invalid JSON arguments: "+err.Error(), true
		return res
	}
	out, err := h(ctx, a, in)
	if err != nil {
		res.Content, res.IsError = err.Error(), true
		return res
	}
	res.Content = out
	return res
}
