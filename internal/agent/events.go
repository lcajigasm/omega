package agent

import (
	"time"

	"github.com/lcajigasm/omega/internal/safety"
)

// EventKind identifies what an Event reports.
type EventKind int

const (
	EvThinking  EventKind = iota // waiting for the model
	EvAssistant                  // model text
	EvToolStart                  // a tool began; Command holds what runs
	EvOutput                     // one line of command output
	EvToolEnd                    // a tool finished
	EvConfirm                    // approval needed; answer on Reply
	EvIdle                       // the model finished its turn; waiting for user input
	EvError                      // a non-fatal error
	EvDone                       // the run ended
)

// Decision is the user's answer to an EvConfirm.
type Decision int

const (
	Deny Decision = iota
	Approve
	ApproveAll // approve this and every later request in this run
)

// Event is sent from the agent to the UI.
type Event struct {
	Kind     EventKind
	Text     string
	Tool     string
	Command  string
	Reason   string
	Risk     safety.Risk
	ExitCode int
	Failed   bool
	Duration time.Duration
	Reply    chan Decision
}
