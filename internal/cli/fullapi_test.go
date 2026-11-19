package cli

import (
	"strings"
	"testing"
)

// The app's plan is the terminal's: the same stages in the same order,
// with the doctor as a report and every other stage as a task.
func TestFullStagesMatchTheGuidedRun(t *testing.T) {
	here := stagesHere()
	got := FullStages()
	if len(got) != len(here) {
		t.Fatalf("%d stages, the guided run has %d", len(got), len(here))
	}
	for i, s := range got {
		if s.ID != here[i].id || s.Command != here[i].command || s.Task != here[i].task || s.Report != (here[i].task == "") {
			t.Errorf("stage %d = %+v, want %s", i, s, here[i].id)
		}
		if s.Title == "" || s.What == "" || s.Why == "" || strings.HasPrefix(s.Title, "full.") {
			t.Errorf("stage %s has no texts: %+v", s.ID, s)
		}
	}
	if got[0].ID != "doctor" || !got[0].Report {
		t.Fatalf("the run starts with %+v", got[0])
	}
}

func TestStageReportOnlyForReports(t *testing.T) {
	var b strings.Builder
	if err := StageReport("update", &b); err == nil {
		t.Fatal("update is a session, not a report")
	}
	if err := StageReport("nope", &b); err == nil {
		t.Fatal("an unknown stage gave a report")
	}
}
