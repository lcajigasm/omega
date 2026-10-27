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
	"github.com/lcajigasm/omega/internal/safety"
)

const maxTurns = 80

// Options configure a run.
type Options struct {
	Model       string
	Approval    string // config.Approve*
	AutoYes     bool   // --yes: approve everything except blocked commands
	ReadOnly    bool   // only Safe actions are allowed
	Interactive bool   // wait for follow-up input after the model finishes
	Timeout     time.Duration
	Config      *config.Config
}

// Agent drives one maintenance session.
type Agent struct {
	opts       Options
	chat       llm.Chat
	events     chan Event
	input      chan string
	approveAll bool
	log        *os.File
	usage      llm.Usage
}

// New creates an agent for the given provider.
func New(p llm.Provider, opts Options) *Agent {
	notes, _ := os.ReadFile(config.NotesPath())
	sys := systemPrompt(string(notes), opts.Config.SkipSteps, opts.ReadOnly)
	a := &Agent{
		opts:   opts,
		chat:   p.NewChat(opts.Model, sys, toolSpecs()),
		events: make(chan Event, 256),
		input:  make(chan string),
	}
	a.openLog(p.Name())
	return a
}

// Events is the stream the UI consumes. It is closed when the run ends.
func (a *Agent) Events() <-chan Event { return a.events }

// Input sends a follow-up message from the user while the agent is idle.
// An empty string ends the session.
func (a *Agent) Input() chan<- string { return a.input }

// Usage returns the tokens spent so far.
func (a *Agent) Usage() llm.Usage { return a.usage }

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

// Run executes the task until the model stops and, in interactive mode, the
// user ends the session.
func (a *Agent) Run(ctx context.Context, task string) {
	defer close(a.events)
	defer a.closeLog()

	text := task
	var results []llm.ToolResult
	for turn := 0; ; turn++ {
		if ctx.Err() != nil {
			a.emit(Event{Kind: EvDone, Text: i18n.T("agent.cancelled")})
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

		a.emit(Event{Kind: EvThinking})
		reply, err := a.chat.Send(ctx, text, results)
		if err != nil {
			if ctx.Err() != nil {
				a.emit(Event{Kind: EvDone, Text: i18n.T("agent.cancelled")})
				return
			}
			a.emit(Event{Kind: EvError, Text: i18n.T("agent.model_error", err.Error())})
			// The failed message was rolled back; let the user retry or quit.
			if !a.opts.Interactive {
				a.emit(Event{Kind: EvDone})
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
		a.usage.InputTokens += reply.Usage.InputTokens
		a.usage.OutputTokens += reply.Usage.OutputTokens
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
		a.emit(Event{Kind: EvDone})
		return false
	}
	a.emit(Event{Kind: EvIdle})
	select {
	case <-ctx.Done():
		a.emit(Event{Kind: EvDone, Text: i18n.T("agent.cancelled")})
		return false
	case msg, ok := <-a.input:
		if !ok || strings.TrimSpace(msg) == "" {
			a.emit(Event{Kind: EvDone})
			return false
		}
		*text = msg
		a.writeLog(Event{Kind: EvAssistant, Text: "> " + msg})
		return true
	}
}

// authorize applies the approval policy and asks the user when needed.
func (a *Agent) authorize(ctx context.Context, tool, command, reason string, risk safety.Risk) (bool, string) {
	if risk == safety.Blocked {
		return false, "Blocked by omega's safety policy; this command can never run."
	}
	if a.opts.ReadOnly && risk > safety.Safe {
		return false, "Read-only mode: this action would change the system and was rejected. Describe it in the summary instead."
	}
	if risk == safety.Safe || a.opts.AutoYes || a.approveAll || a.opts.Approval == config.ApproveNever {
		return true, ""
	}
	if risk == safety.Normal && a.opts.Approval != config.ApproveAll {
		return true, ""
	}
	reply := make(chan Decision, 1)
	a.emit(Event{Kind: EvConfirm, Tool: tool, Command: command, Reason: reason, Risk: risk, Reply: reply})
	select {
	case <-ctx.Done():
		return false, "cancelled"
	case d := <-reply:
		switch d {
		case ApproveAll:
			a.approveAll = true
			return true, ""
		case Approve:
			return true, ""
		}
	}
	return false, "The user denied this action. Do not retry it or an equivalent; carry on with the rest."
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
