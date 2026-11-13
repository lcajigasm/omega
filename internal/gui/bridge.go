// Package gui connects an agent session to a graphical frontend. It turns
// the agent's events into plain messages a webview can read and routes the
// user's answers back to the request that asked for them. It does not know
// which toolkit draws the window.
package gui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/ui"
)

// Agent is what the bridge needs from a session. *agent.Agent satisfies it.
type Agent interface {
	Events() <-chan agent.Event
	Input() chan<- string
	Mode() agent.Mode
	SetMode(agent.Mode)
}

// Choice is one option of a selection card.
type Choice struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
	Impact string `json:"impact,omitempty"`
	Risk   string `json:"risk,omitempty"`
}

// Stats are the running totals of the session.
type Stats struct {
	Model     string  `json:"model"`
	Input     int64   `json:"input"`
	Output    int64   `json:"output"`
	Cost      float64 `json:"cost"`
	CostKnown bool    `json:"costKnown"`
	Seconds   float64 `json:"seconds"`
}

// Message is one event as the frontend receives it.
//
// ID identifies a tool call (tool_start, output, tool_end) or a request the
// user has to answer (confirm, choice, password). Answers name that ID.
type Message struct {
	Kind string `json:"kind"`
	// Seq orders the messages of a session, from 1. A window that is drawn
	// again asks for the ones it missed and uses it to drop a repeat.
	Seq      int      `json:"seq,omitempty"`
	ID       int      `json:"id,omitempty"`
	Text     string   `json:"text,omitempty"`
	HTML     string   `json:"html,omitempty"` // Text rendered from Markdown (assistant)
	Tool     string   `json:"tool,omitempty"` // display name: Bash, Step, Read…
	Arg      string   `json:"arg,omitempty"`
	Command  string   `json:"command,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Risk     string   `json:"risk,omitempty"`
	Always   bool     `json:"always,omitempty"`
	Failed   bool     `json:"failed,omitempty"`
	ExitCode int      `json:"exitCode,omitempty"`
	Seconds  float64  `json:"seconds,omitempty"`
	Lines    int      `json:"lines,omitempty"`
	Choices  []Choice `json:"choices,omitempty"`
	Multi    bool     `json:"multi,omitempty"`
	Mode     string   `json:"mode,omitempty"`
	Stats    *Stats   `json:"stats,omitempty"`
}

var kinds = map[agent.EventKind]string{
	agent.EvThinking: "thinking", agent.EvAssistant: "assistant", agent.EvToolStart: "tool_start",
	agent.EvOutput: "output", agent.EvToolEnd: "tool_end", agent.EvConfirm: "confirm",
	agent.EvChoice: "choice", agent.EvPassword: "password", agent.EvDenied: "denied",
	agent.EvModel: "model", agent.EvMode: "mode", agent.EvUsage: "usage", agent.EvIdle: "idle",
	agent.EvError: "error", agent.EvDone: "done",
}

var modes = [...]string{"ask", "auto", "readonly"}

// ModeName is the name the frontend uses for an approval mode.
func ModeName(m agent.Mode) string { return modes[m] }

// ErrNoRequest is returned when an answer names a request that is not
// waiting: it was answered already or the session moved on.
var ErrNoRequest = errors.New("no such pending request")

// Bridge carries one session.
type Bridge struct {
	ag   Agent
	ctx  context.Context
	emit func(Message)

	mu      sync.Mutex
	nextID  int
	pending map[int]agent.Event // confirm, choice and password requests
	running map[string][]int    // tool calls in progress, by tool and command
	idle    bool
}

// New creates the bridge. emit is called once per message: from Run's
// goroutine, in order, for what the agent does, and from the caller's for
// the "answered" that follows an answer. It must be safe to call from both.
func New(ctx context.Context, ag Agent, emit func(Message)) *Bridge {
	return &Bridge{ag: ag, ctx: ctx, emit: emit, pending: map[int]agent.Event{}, running: map[string][]int{}}
}

// Run forwards events until the session ends, then emits "closed".
func (b *Bridge) Run() {
	for e := range b.ag.Events() {
		if m, ok := b.translate(e); ok {
			b.emit(m)
		}
	}
	b.mu.Lock()
	b.pending = map[int]agent.Event{}
	b.idle = false
	b.mu.Unlock()
	b.emit(Message{Kind: "closed"})
}

func toolKey(e agent.Event) string { return e.Tool + "\x00" + e.Command }

func (b *Bridge) translate(e agent.Event) (Message, bool) {
	kind, ok := kinds[e.Kind]
	if !ok {
		return Message{}, false
	}
	m := Message{Kind: kind, Text: e.Text}
	if e.Stats.Model != "" {
		m.Stats = &Stats{Model: e.Stats.Model, Input: e.Stats.Usage.Input(), Output: e.Stats.Usage.OutputTokens,
			Cost: e.Stats.Cost, CostKnown: e.Stats.CostKnown, Seconds: e.Stats.Elapsed.Seconds()}
	}
	if e.Tool != "" || e.Command != "" {
		m.Tool, m.Arg = ui.ToolLabel(e.Tool, e.Command)
		m.Command = e.Command
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.idle = e.Kind == agent.EvIdle
	switch e.Kind {
	case agent.EvAssistant:
		m.HTML = Markdown(e.Text)
	case agent.EvToolStart:
		b.nextID++
		m.ID = b.nextID
		b.running[toolKey(e)] = append(b.running[toolKey(e)], m.ID)
	case agent.EvOutput:
		// Output belongs to the latest call of that tool still running.
		if ids := b.running[toolKey(e)]; len(ids) > 0 {
			m.ID = ids[len(ids)-1]
		} else {
			return Message{}, false
		}
	case agent.EvToolEnd:
		k := toolKey(e)
		if ids := b.running[k]; len(ids) > 0 {
			m.ID = ids[len(ids)-1]
			if b.running[k] = ids[:len(ids)-1]; len(ids) == 1 {
				delete(b.running, k)
			}
		} else {
			// A tool that reported no start: the frontend adds it finished.
			b.nextID++
			m.ID = b.nextID
		}
		m.Failed, m.ExitCode, m.Lines, m.Reason = e.Failed, e.ExitCode, e.Lines, e.Reason
		m.Seconds = e.Duration.Seconds()
	case agent.EvDenied:
		m.Reason = e.Reason
	case agent.EvConfirm:
		m.ID = b.ask(e)
		m.Reason, m.Risk, m.Always = e.Reason, e.Risk.String(), e.Always
	case agent.EvChoice:
		m.ID = b.ask(e)
		m.Multi = e.Multi
		for _, c := range e.Choices {
			m.Choices = append(m.Choices, Choice{ID: c.ID, Label: c.Label, Detail: c.Detail, Impact: c.Impact, Risk: c.Risk})
		}
	case agent.EvPassword:
		m.ID = b.ask(e)
		m.Failed = e.Failed // the previous password was wrong
	case agent.EvMode:
		m.Mode = ModeName(e.Mode)
	}
	return m, true
}

// ask records a request that waits for the user. b.mu is held.
func (b *Bridge) ask(e agent.Event) int {
	b.nextID++
	b.pending[b.nextID] = e
	return b.nextID
}

// take removes and returns a pending request of the wanted kind.
func (b *Bridge) take(id int, kind agent.EventKind) (agent.Event, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.pending[id]
	if !ok || e.Kind != kind {
		return agent.Event{}, ErrNoRequest
	}
	delete(b.pending, id)
	return e, nil
}

// Confirm answers an approval request. decision is "approve", "all" or
// anything else for no. "all" on a request that cannot be skipped approves
// that one only.
func (b *Bridge) Confirm(id int, decision string) error {
	e, err := b.take(id, agent.EvConfirm)
	if err != nil {
		return err
	}
	d := agent.Deny
	switch {
	case decision == "approve", decision == "all" && e.Always:
		d = agent.Approve
	case decision == "all":
		d = agent.ApproveAll
	}
	e.Reply <- d // buffered by the agent
	b.emit(Message{Kind: "answered", ID: id, Text: decision})
	return nil
}

// Choose answers a selection card with the picked IDs; none skips it. IDs
// the card did not offer are dropped, and a single-choice card keeps one.
func (b *Bridge) Choose(id int, picks []string) error {
	e, err := b.take(id, agent.EvChoice)
	if err != nil {
		return err
	}
	offered := map[string]bool{}
	for _, c := range e.Choices {
		offered[c.ID] = true
	}
	var ids []string
	for _, p := range picks {
		if offered[p] {
			ids = append(ids, p)
			offered[p] = false
		}
	}
	if !e.Multi && len(ids) > 1 {
		ids = ids[:1]
	}
	e.Picks <- ids
	b.emit(Message{Kind: "answered", ID: id, Text: strings.Join(ids, "\x00")})
	return nil
}

// Password answers a sudo prompt. An empty secret skips what needs
// administrator access. The secret is handed to the agent and kept nowhere.
func (b *Bridge) Password(id int, secret string) error {
	e, err := b.take(id, agent.EvPassword)
	if err != nil {
		return err
	}
	e.Secret <- secret
	// Whether one was given, never the password.
	given := ""
	if secret != "" {
		given = "given"
	}
	b.emit(Message{Kind: "answered", ID: id, Text: given})
	return nil
}

// ErrBusy is returned when a message is sent while the agent is working.
var ErrBusy = errors.New("the agent is not waiting for a message")

// Send gives the agent a follow-up message. It is accepted only while the
// agent waits for one.
func (b *Bridge) Send(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	b.mu.Lock()
	if !b.idle {
		b.mu.Unlock()
		return ErrBusy
	}
	b.idle = false
	b.mu.Unlock()
	select {
	case b.ag.Input() <- text:
		return nil
	case <-b.ctx.Done():
		return b.ctx.Err()
	}
}

// SetMode changes the approval mode: "ask", "auto" or "readonly".
func (b *Bridge) SetMode(name string) error {
	for i, n := range modes {
		if n == name {
			b.ag.SetMode(agent.Mode(i))
			return nil
		}
	}
	return errors.New("unknown mode: " + name)
}

// Mode is the current approval mode.
func (b *Bridge) Mode() string { return ModeName(b.ag.Mode()) }

var md = goldmark.New(
	// GFM, with the alignment of a table cell as an attribute: the window
	// allows no inline style.
	goldmark.WithExtensions(extension.Linkify, extension.Strikethrough, extension.TaskList,
		extension.NewTable(extension.WithTableCellAlignMethod(extension.TableCellAlignAttribute))),
	goldmark.WithRendererOptions(renderer.WithNodeRenderers(util.Prioritized(noImages{}, 100))))

// noImages draws an image as its text. The window fetches an image as soon
// as it is drawn, with no click, and the address is the model's: it could
// carry what the model has read to another machine, or name one of the
// app's own addresses and act as the user.
type noImages struct{}

func (noImages) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindImage, func(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			img := n.(*ast.Image)
			text := string(img.Text(source)) //nolint:staticcheck // the alt text, as the default renderer reads it
			if text == "" {
				text = string(img.Destination)
			}
			_, _ = w.Write(util.EscapeHTML([]byte(text)))
		}
		return ast.WalkSkipChildren, nil
	})
}

// Markdown renders model text as HTML. Raw HTML in the text is not passed
// through, and an image is shown as its text: the model's words are shown,
// never run and never fetched.
func Markdown(s string) string {
	var out bytes.Buffer
	if err := md.Convert([]byte(s), &out); err != nil {
		return ""
	}
	return out.String()
}
