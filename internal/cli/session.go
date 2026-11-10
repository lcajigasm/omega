package cli

import (
	"context"

	"github.com/lcajigasm/omega/internal/agent"
)

// SessionRequest is what a frontend other than the terminal asks for.
type SessionRequest struct {
	Task   string // one of agent.Task*; ignored when Prompt is set
	Prompt string // free text instead of a preset task
	DryRun bool   // read only: changes are offered, never made
	Demo   bool   // scripted session: no API calls, no real commands
}

// Session is a run that has started. Its agent is already working; the
// frontend consumes Agent.Events and answers on the channels they carry.
type Session struct {
	Agent    *agent.Agent
	Provider string
	Model    string
	Escalate string
	Models   []string
	Task     string
}

// StartSession starts the agent for a frontend that is not a terminal, such
// as the desktop app. Init must have been called. The session is always
// interactive: the frontend can answer and send follow-ups. Cancelling ctx
// stops it.
func StartSession(ctx context.Context, req SessionRequest) (*Session, error) {
	r, err := prepareRun(req.Task, runFlags{prompt: req.Prompt, dryRun: req.DryRun, demo: req.Demo})
	if err != nil {
		return nil, err
	}
	ag := r.agent(true, true, "")
	go ag.Run(ctx, r.prompt)
	return &Session{Agent: ag, Provider: r.p.Name(), Model: r.plan.Model, Escalate: r.plan.Escalate,
		Models: r.plan.Choices, Task: r.task}, nil
}

// Version is the version Init was given.
func Version() string { return version }
