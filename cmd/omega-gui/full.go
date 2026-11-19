//go:build gui

package main

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/cli"
	"github.com/lcajigasm/omega/internal/gui"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/ui"
)

// The guided run, `omega full` in the window: the stages that apply to this
// machine, each one a session of its own that ends when its work does, and
// between two of them the choice the terminal offers: go on, skip the next,
// repeat the last one when it failed, or finish there.

// fullRun is a guided run in progress. Its fields are guarded by App.mu.
type fullRun struct {
	stages []gui.Stage
	info   []cli.StageInfo // the same stages: their task, or that they are a report
	dryRun bool
	cur    int  // the stage running; -1 between two
	last   int  // the stage that ran last; -1 before the first
	waits  bool // the run waits for the user between two stages
	// stop ends the stage that is running; stopped says the user did.
	stop    context.CancelFunc
	stopped bool
	// mode is the approval mode the user chose during the run, kept for
	// the stages after it; "" until they choose one.
	mode string
	// next carries what the user decided between two stages, and "plan"
	// when the plan changed while the run waited.
	next chan string
}

// upcoming is the first ticked stage that has not run; -1 when none is left.
func (f *fullRun) upcoming() int {
	for i, s := range f.stages {
		if s.Selected && s.Status == ui.StagePending {
			return i
		}
	}
	return -1
}

// ordinal is the position of stage i among the stages of the run, and how
// many there are.
func (f *fullRun) ordinal(i int) (n, total int) {
	for j, s := range f.stages {
		if s.Selected || (s.Status != ui.StagePending && s.Status != ui.StageSkipped) {
			total++
			if j <= i {
				n++
			}
		}
	}
	return n, total
}

// FullPlan lists the stages of the guided run that apply to this machine.
func (a *App) FullPlan() []cli.StageInfo { return cli.FullStages() }

// StartFull begins the guided run with the stages named in ids, in their
// own order; no ids means every stage. dryRun makes every stage read only.
func (a *App) StartFull(ids []string, dryRun bool) (SessionInfo, error) {
	info := cli.FullStages()
	pick := map[string]bool{}
	for _, id := range ids {
		pick[id] = true
	}
	f := &fullRun{info: info, dryRun: dryRun, cur: -1, last: -1, next: make(chan string, 1)}
	picked := false
	for _, s := range info {
		on := len(ids) == 0 || pick[s.ID]
		picked = picked || on
		f.stages = append(f.stages, gui.Stage{ID: s.ID, Title: s.Title, What: s.What, Why: s.Why, Command: s.Command,
			Risk: s.Risk, Selected: on, Status: ui.StagePending})
	}
	if !picked {
		return SessionInfo{}, local(errNoStages)
	}

	a.mu.Lock()
	if a.bridge != nil || a.starting || a.full != nil {
		a.mu.Unlock()
		return SessionInfo{}, local(errRunning)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	a.full, a.cancel, a.done = f, cancel, done
	a.seq, a.log, a.prompt = 0, nil, ""
	a.title = SessionInfo{Task: agent.TaskFull}
	title := a.title
	a.mu.Unlock()

	a.sendStages("stages", -1)
	go a.runFull(ctx, f, done)
	return title, nil
}

// sendStages tells the window how every stage stands. i is the stage the
// message is about, -1 for none.
func (a *App) sendStages(kind string, i int) {
	a.mu.Lock()
	stages := append([]gui.Stage(nil), a.full.stages...)
	a.mu.Unlock()
	a.send(gui.Message{Kind: kind, Stage: i + 1, Stages: stages})
}

// runFull runs the stages and the choices between them, then the summary.
func (a *App) runFull(ctx context.Context, f *fullRun, done chan struct{}) {
	defer func() {
		a.sendStages("summary", -1)
		a.mu.Lock()
		a.full, a.cancel = nil, nil
		a.mu.Unlock()
		a.send(gui.Message{Kind: "closed"})
		close(done)
	}()
	a.mu.Lock()
	first := f.upcoming()
	a.mu.Unlock()
	if first < 0 {
		return
	}
	a.runStage(ctx, f, first)
	for ctx.Err() == nil {
		a.mu.Lock()
		n := f.upcoming()
		retry := f.last >= 0 && (f.stages[f.last].Status == ui.StageFailed || f.stages[f.last].Status == ui.StageStopped)
		f.waits = n >= 0
		a.mu.Unlock()
		if n < 0 {
			return
		}
		a.send(gui.Message{Kind: "between", Stage: n + 1, Retry: retry})
		var act string
		select {
		case act = <-f.next:
		case <-ctx.Done():
			return
		}
		a.mu.Lock()
		f.waits = false
		a.mu.Unlock()
		switch act {
		case "go":
			a.runStage(ctx, f, n)
		case "retry":
			if retry {
				a.runStage(ctx, f, f.last)
			}
		case "skip":
			a.mu.Lock()
			f.stages[n].Status = ui.StageSkipped
			a.mu.Unlock()
			a.sendStages("stages", -1)
		case "end":
			return
		}
		// "plan": the plan changed while the run waited; the next stage
		// is looked up again.
	}
}

// runStage runs stage i: its report, or a session that ends with its work.
func (a *App) runStage(ctx context.Context, f *fullRun, i int) {
	sctx, stop := context.WithCancel(ctx)
	defer stop()
	a.mu.Lock()
	f.cur, f.last, f.stop, f.stopped = i, i, stop, false
	f.stages[i] = gui.Stage{ID: f.stages[i].ID, Title: f.stages[i].Title, What: f.stages[i].What, Why: f.stages[i].Why,
		Command: f.stages[i].Command, Risk: f.stages[i].Risk, Selected: true, Status: ui.StageRunning}
	n, total := f.ordinal(i)
	info, dryRun, mode := f.info[i], f.dryRun, f.mode
	a.mu.Unlock()
	a.sendStages("stages", i)
	a.send(gui.Message{Kind: "stage_start", Stage: i + 1, Text: i18n.T("full.stage", n, total)})

	began := time.Now()
	var stats *gui.Stats
	var note string
	failed := false
	if info.Report {
		failed, note = a.stageReport(info)
	} else {
		failed, note, stats = a.stageSession(sctx, info, dryRun, mode)
	}

	a.mu.Lock()
	s := &f.stages[i]
	s.Seconds = time.Since(began).Seconds()
	if stats != nil {
		s.Cost, s.CostKnown = stats.Cost, stats.CostKnown
	}
	switch {
	case f.stopped || ctx.Err() != nil:
		s.Status = ui.StageStopped
	case failed:
		s.Status, s.Note = ui.StageFailed, note
	default:
		s.Status = ui.StageDone
	}
	f.cur, f.stop = -1, nil
	a.mu.Unlock()
	a.sendStages("stage_end", i)
}

// stageReport runs a stage that writes a report, shown as one call.
func (a *App) stageReport(info cli.StageInfo) (failed bool, note string) {
	tool, arg := "omega", strings.TrimPrefix(info.Command, "omega ")
	a.send(gui.Message{Kind: "tool_start", ID: 1, Tool: tool, Arg: arg, Command: info.Command})
	var out bytes.Buffer
	began := time.Now()
	err := cli.StageReport(info.ID, &out)
	lines := 0
	for _, l := range strings.Split(strings.TrimRight(out.String(), "\n"), "\n") {
		if l != "" || lines > 0 {
			a.send(gui.Message{Kind: "output", ID: 1, Tool: tool, Text: l})
			lines++
		}
	}
	end := gui.Message{Kind: "tool_end", ID: 1, Tool: tool, Arg: arg, Command: info.Command, Lines: lines,
		Seconds: time.Since(began).Seconds()}
	if err != nil {
		end.Failed, end.ExitCode, end.Reason = true, 1, err.Error()
		note = err.Error()
	}
	a.send(end)
	return err != nil, note
}

// stageSession runs a stage that is a session of the agent, and returns
// when it has ended.
func (a *App) stageSession(ctx context.Context, info cli.StageInfo, dryRun bool, mode string) (failed bool, note string, stats *gui.Stats) {
	a.mu.Lock()
	demo := a.demo
	a.mu.Unlock()
	s, err := cli.StartSession(ctx, cli.SessionRequest{Task: info.Task, DryRun: dryRun, Demo: demo, Stage: true})
	if err != nil {
		a.send(gui.Message{Kind: "error", Text: local(err).Error()})
		return true, local(err).Error(), nil
	}
	// The stage's own messages go to the window, but for the end of its
	// session: the run goes on. Its last totals and error are kept.
	emit := func(m gui.Message) {
		if m.Kind == "closed" {
			return
		}
		if m.Stats != nil {
			stats = m.Stats
		}
		if m.Kind == "error" && strings.TrimSpace(m.Text) != "" {
			note = strings.TrimSpace(m.Text)
		}
		a.send(m)
	}
	b := gui.New(ctx, s.Agent, emit)
	if mode != "" {
		_ = b.SetMode(mode)
	}
	a.mu.Lock()
	a.bridge = b
	a.title.Provider, a.title.Model, a.title.Mode = s.Provider, s.Model, b.Mode()
	a.mu.Unlock()
	a.send(gui.Message{Kind: "session", Text: s.Provider + " · " + s.Model, Mode: b.Mode()})
	b.Run()
	a.mu.Lock()
	if a.bridge == b {
		a.bridge = nil
	}
	a.mu.Unlock()
	code := s.Agent.ExitCode()
	return code != 0 && code != agent.ExitCancelled, note, stats
}

// FullNext answers the choice between two stages: "go", "skip" (the next
// one), "retry" (the last one) or "end".
func (a *App) FullNext(action string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	f := a.full
	if f == nil || !f.waits {
		return local(errNoSession)
	}
	switch action {
	case "go", "skip", "retry", "end":
	default:
		return local(errNoSession)
	}
	select {
	case f.next <- action:
		f.waits = false
	default:
	}
	return nil
}

// FullToggle ticks or unticks a stage that has not run, while the run goes
// on: the plan can change at any time, as in the terminal.
func (a *App) FullToggle(id string, on bool) error {
	a.mu.Lock()
	f := a.full
	if f == nil {
		a.mu.Unlock()
		return local(errNoSession)
	}
	changed := false
	for i := range f.stages {
		s := &f.stages[i]
		if s.ID == id && (s.Status == ui.StagePending || s.Status == ui.StageSkipped) {
			s.Selected, s.Status, changed = on, ui.StagePending, true
		}
	}
	if changed && f.waits {
		// The choice on screen named a stage that may have changed: the
		// run looks for the next one again and asks anew.
		select {
		case f.next <- "plan":
			f.waits = false
		default:
		}
	}
	a.mu.Unlock()
	if changed {
		a.sendStages("stages", -1)
	}
	return nil
}
