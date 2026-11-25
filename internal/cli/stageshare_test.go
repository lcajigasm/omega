package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/ui"
)

// A stage of a run is told what the stages before it concluded; a task on
// its own, or a question in the user's words, is not.
func TestStagesHandOnWhatTheyConcluded(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	share := newStageShare()
	share.add("analyze", "Disk at 91%; 6 Homebrew packages outdated.", false)
	share.add("health", "", false) // said nothing: nothing to hand on

	r, err := prepareRun("update", runFlags{demo: true, stage: true, share: share})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.prompt, "<earlier_stages>") || !strings.Contains(r.prompt, "## analyze\nDisk at 91%") || strings.Contains(r.prompt, "## health") {
		t.Errorf("the stage was not told about the earlier ones:\n%s", r.prompt)
	}
	for _, f := range []runFlags{{demo: true, share: share}, {demo: true, stage: true}, {demo: true, stage: true, share: share, prompt: "why is it slow"}} {
		r, err := prepareRun("update", f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(r.prompt, "<earlier_stages>") {
			t.Errorf("%+v: told about stages it is not part of", f)
		}
	}

	// Anything outside the sessions that may change the machine empties
	// the cache the stages share; a task on its own has no share at all.
	share.changed()
	if share.resultCache().Len() != 0 {
		t.Error("the shared cache outlived a change")
	}
	var none *stageShare
	none.add("x", "y", false)
	none.changed()
	if none.prompt() != "" || none.resultCache() != nil {
		t.Error("a task on its own has nothing shared")
	}
}

// Without anybody watching, every stage's line says when it started: the
// output ends in a log read later.
func TestUnattendedLinesSayWhen(t *testing.T) {
	old := clock
	t.Cleanup(func() { clock = old })
	clock = func() time.Time { return time.Date(2026, 11, 25, 9, 12, 5, 0, time.Local) }
	stages := []ui.GuideStage{{ID: "update", Title: "Update", Command: "omega update", Selected: true}}
	var b strings.Builder
	runFull(&b, stages, func(string) error { return nil }, nil, nil)
	if !strings.Contains(b.String(), "omega update · 09:12:05") {
		t.Errorf("no time on the stage line:\n%s", b.String())
	}
	b.Reset()
	runFull(&b, stages, func(string) error { return nil }, func() string { return "" }, nil)
	if strings.Contains(b.String(), "09:12:05") {
		t.Errorf("someone is watching: no clock needed:\n%s", b.String())
	}
}
