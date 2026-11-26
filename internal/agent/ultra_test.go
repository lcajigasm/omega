package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/deepclean"
)

// fakeDeep replaces the deep scan with two findings and records what was
// removed.
func fakeDeep(t *testing.T) *[]string {
	t.Helper()
	var removed []string
	oldScan, oldApply, oldRoots := deepScan, deepApply, deepRoots
	deepRoots = func() ([]string, []string) { return nil, nil }
	deepScan = func(context.Context, deepclean.Options) deepclean.Report {
		return deepclean.Report{Findings: []deepclean.Finding{
			{ID: "wt-1", Kind: deepclean.KindWorktree, Title: "app · wt-1", Size: 2e9, Verdict: deepclean.Safe, Action: "git worktree remove wt-1"},
			{ID: "wt-2", Kind: deepclean.KindWorktree, Title: "omega · mine", Verdict: deepclean.Keep, Reasons: []string{"omega is running from inside it"}},
		}}
	}
	deepApply = func(_ context.Context, f deepclean.Finding) (deepclean.Result, error) {
		removed = append(removed, f.ID)
		return deepclean.Result{ID: f.ID, Freed: f.Size}, nil
	}
	t.Cleanup(func() { deepScan, deepApply, deepRoots = oldScan, oldApply, oldRoots })
	return &removed
}

func TestUltraToolsExistOnlyInUltra(t *testing.T) {
	sandbox(t)
	fakeDeep(t)
	res, _, _ := drive(t, Options{}, Approve, call("1", "deep_scan", nil))
	if !res[0].IsError {
		t.Fatalf("deep_scan outside ultra: %+v", res[0])
	}
	res, _, _ = drive(t, Options{Ultra: true}, Approve, call("1", "deep_scan", nil))
	if res[0].IsError || !strings.Contains(res[0].Content, `"wt-1"`) {
		t.Fatalf("deep_scan in ultra: %+v", res[0])
	}
}

// Every removal is put to the person, also under --yes and with the
// approval policy at never; a kept finding is never removed.
func TestDeepCleanAsksForEachAndNeverRemovesWhatIsKept(t *testing.T) {
	sandbox(t)
	removed := fakeDeep(t)
	scan := call("1", "deep_scan", nil)
	clean := call("2", "deep_clean", map[string]any{"ids": []string{"wt-1", "wt-2", "nope"}})
	res, _, asked := drive(t, Options{Ultra: true, AutoYes: true}, Approve, scan, clean)
	if asked != 1 || strings.Join(*removed, ",") != "wt-1" {
		t.Fatalf("asked %d, removed %v; want one question and wt-1 only", asked, *removed)
	}
	got := res[1].Content
	if !strings.Contains(got, "wt-2 (omega · mine): kept") || !strings.Contains(got, "nope: not a finding") {
		t.Fatalf("result: %s", got)
	}

	*removed = nil
	_, _, asked = drive(t, Options{Ultra: true}, Deny, scan, clean)
	if asked != 1 || len(*removed) != 0 {
		t.Fatalf("denied: asked %d, removed %v", asked, *removed)
	}
}

func TestOmegaCmdAsksBeforeEveryChangeAndRunsReadsDirectly(t *testing.T) {
	sandbox(t)
	var ran []string
	old := OmegaRun
	OmegaRun = func(_ context.Context, args ...string) (string, error) {
		ran = append(ran, strings.Join(args, " "))
		return "ok", nil
	}
	t.Cleanup(func() { OmegaRun = old })

	_, _, asked := drive(t, Options{Ultra: true, AutoYes: true}, Approve,
		call("1", "omega_cmd", map[string]any{"action": "git_status"}),
		call("2", "omega_cmd", map[string]any{"action": "git_clean"}),
		call("3", "omega_cmd", map[string]any{"action": "assist_apply", "check": "claude-code-cleanup", "value": "14"}))
	if asked != 2 {
		t.Errorf("asked %d times, want 2 (the two changes)", asked)
	}
	want := "git status --json|git clean --yes|assist apply claude-code-cleanup 14 --yes"
	if strings.Join(ran, "|") != want {
		t.Errorf("ran %q, want %q", strings.Join(ran, "|"), want)
	}

	ran = nil
	res, _, _ := drive(t, Options{Ultra: true}, Approve,
		call("1", "omega_cmd", map[string]any{"action": "assist_apply", "check": "x; rm -rf ~"}),
		call("2", "omega_cmd", map[string]any{"action": "rm"}))
	if len(ran) != 0 || !res[0].IsError || !res[1].IsError {
		t.Fatalf("a value that is not plain, or an unknown action, must be refused: ran %v, %+v", ran, res)
	}
}

// An option that changes something without saying what verified it is
// refused in ultra, and shown with its evidence when it has one.
func TestUltraCardsNeedEvidence(t *testing.T) {
	s := runScript(t, Options{Ultra: true, Interactive: true, OneShot: true}, Approve, []string{"a"}, nil,
		proposal(true, opt("a", "Remove the worktree", "dangerous", true), opt("look", "Read the logs", "read-only", false)))
	if got := s.p.results[1][0]; !got.IsError || !strings.Contains(got.Content, "evidence") || !strings.Contains(got.Content, "a") {
		t.Fatalf("a card without evidence was shown: %+v", got)
	}

	withEvidence := opt("a", "Remove the worktree", "dangerous", true)
	withEvidence["evidence"] = "PR #12 merged, branch pushed, only PLAN.md untracked"
	s = runScript(t, Options{Ultra: true, Interactive: true, OneShot: true}, Approve, []string{"a"}, nil, proposal(true, withEvidence))
	if got := s.p.results[1][0]; got.IsError {
		t.Fatalf("a card with evidence was refused: %+v", got)
	}
	shown := false
	for _, e := range s.events {
		if e.Kind == EvChoice && len(e.Choices) == 1 && strings.Contains(e.Choices[0].Detail, "PR #12 merged") {
			shown = true
		}
	}
	if !shown {
		t.Errorf("the evidence is not on the card the person answers")
	}

	// Outside ultra nothing changes.
	s = runScript(t, Options{Interactive: true, OneShot: true}, Approve, []string{"a"}, nil,
		proposal(true, opt("a", "Remove the worktree", "dangerous", true)))
	if got := s.p.results[1][0]; got.IsError {
		t.Fatalf("an ordinary run refused a card: %+v", got)
	}
}

// Ultra has no automatic mode: not from the app's mode switch, not from
// "approve all" on a card.
func TestUltraNeverGoesAutomatic(t *testing.T) {
	sandbox(t)
	fakeDeep(t)
	scan := call("1", "deep_scan", nil)
	clean := call("2", "omega_cmd", map[string]any{"action": "git_gc"})
	again := call("3", "omega_cmd", map[string]any{"action": "git_gc"})
	old := OmegaRun
	OmegaRun = func(context.Context, ...string) (string, error) { return "ok", nil }
	t.Cleanup(func() { OmegaRun = old })
	_, _, asked := drive(t, Options{Ultra: true}, ApproveAll, scan, clean, again)
	if asked != 2 {
		t.Errorf("after approve all, ultra must still ask: asked %d times, want 2", asked)
	}

	// An agent that opens no transcript: Windows cannot remove a file
	// left open.
	a := &Agent{opts: Options{Ultra: true}, mode: ModeAsk}
	a.SetMode(ModeAuto)
	if a.Mode() == ModeAuto {
		t.Errorf("ultra switched to automatic mode")
	}
}
