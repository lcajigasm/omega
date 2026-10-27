package agent

import (
	"time"

	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/safety"
)

// EventKind identifies what an Event reports.
type EventKind int

const (
	EvThinking  EventKind = iota // waiting for the model
	EvAssistant                  // model text (Markdown)
	EvToolStart                  // a tool began; Command holds what runs
	EvOutput                     // one line of command output
	EvToolEnd                    // a tool finished
	EvConfirm                    // approval needed; answer on Reply
	EvChoice                     // selection card; answer on Picks
	EvPassword                   // sudo needs the password; answer on Secret ("" cancels)
	EvDenied                     // an action was refused (policy or user)
	EvModel                      // switched to another model (Text)
	EvMode                       // read-only mode lifted
	EvUsage                      // token and cost totals changed
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

// Choice is one option of a selection card.
type Choice struct {
	ID     string
	Label  string
	Detail string
	Impact string // e.g. "~9 GB"
	Risk   string // "read-only", "normal" or "dangerous"
}

// Stats are the running totals of a session.
type Stats struct {
	Usage     llm.Usage
	Cost      float64
	CostKnown bool
	Model     string
	Elapsed   time.Duration
}

// Event is sent from the agent to the UI.
type Event struct {
	Kind     EventKind
	Text     string
	Tool     string
	Command  string
	Reason   string
	Risk     safety.Risk
	Always   bool // confirmation cannot be skipped with "approve all"
	ExitCode int
	Failed   bool
	Duration time.Duration
	Lines    int
	Reply    chan Decision
	Choices  []Choice
	Multi    bool
	Picks    chan []string
	Secret   chan string
	Stats    Stats
}
