package cli

import (
	"fmt"
	"io"

	"github.com/lcajigasm/omega/internal/i18n"
)

// What the desktop app needs for the guided run, `omega full`: the stages
// that apply to this machine and the report of a stage that is not a
// session. The app runs them in turn, as the terminal does.

// StageInfo is one stage of the guided run, as the plan shows it.
type StageInfo struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	What    string `json:"what"`
	Why     string `json:"why"`
	Command string `json:"command"` // what starts it on its own
	Risk    string `json:"risk"`    // read-only, normal or dangerous
	// Report says the stage writes a report instead of starting a session.
	Report bool `json:"report"`
	// Task is the agent task the stage starts; "" for a report.
	Task string `json:"task"`
}

// FullStages lists the stages of the guided run that apply to this machine,
// in the order they run.
func FullStages() []StageInfo {
	out := []StageInfo{}
	for _, s := range stagesHere() {
		g := guideStage(s, true)
		out = append(out, StageInfo{ID: s.id, Title: g.Title, What: g.What, Why: g.Why, Command: s.command,
			Risk: s.risk, Report: s.task == "", Task: s.task})
	}
	return out
}

// StageReport writes the report of a stage that is not a session.
func StageReport(id string, w io.Writer) error {
	for _, s := range stagesHere() {
		if s.id == id && s.task == "" {
			return s.report(w)
		}
	}
	return fmt.Errorf("%s", i18n.T("full.unknown_stage", id, stageIDList()))
}
