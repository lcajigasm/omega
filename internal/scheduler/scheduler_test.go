package scheduler

import (
	"context"
	"testing"
)

func TestEscapeAppleScript(t *testing.T) {
	in := `Hello "World" \ test`
	got := escapeAppleScript(in)
	want := `Hello \"World\" \\ test`
	if got != want {
		t.Errorf("escapeAppleScript(%q) = %q; want %q", in, got, want)
	}
}

func TestStatus(t *testing.T) {
	st, err := Status()
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if st == nil {
		t.Fatalf("expected non-nil ScheduleStatus")
	}
}

func TestNotify(t *testing.T) {
	// Smoke test
	err := Notify(context.Background(), "Omega Test", "Unit test notification")
	if err != nil {
		t.Errorf("Notify returned error: %v", err)
	}
}
