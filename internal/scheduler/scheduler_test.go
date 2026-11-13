package scheduler

import (
	"context"
	"strings"
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

func TestValidate(t *testing.T) {
	if err := Validate("monthly", "quick", 3, 0); err != nil {
		t.Errorf("monthly: %v", err)
	}
	for _, bad := range []struct {
		interval, task string
		hour, minute   int
	}{
		{"hourly", "quick", 3, 0}, {"weekly", "bogus", 3, 0}, {"weekly", "quick", 99, 0},
		{"weekly", "quick", -1, 0}, {"weekly", "quick", 3, 60}, {"weekly", "quick --sudo", 3, 0},
	} {
		if err := Validate(bad.interval, bad.task, bad.hour, bad.minute); err == nil {
			t.Errorf("%+v was accepted", bad)
		}
	}
}

// monthly is monthly on the three systems, not weekly under another name.
func TestMonthlySchedule(t *testing.T) {
	if c := macCalendar("monthly", 4, 5); !strings.Contains(c, "<key>Day</key>") || strings.Contains(c, "Weekday") {
		t.Errorf("launchd: %s", c)
	}
	if c := macCalendar("daily", 4, 5); strings.Contains(c, "Day") {
		t.Errorf("launchd daily: %s", c)
	}
	if c := systemdCalendar("monthly", 4, 5); c != "*-*-01 04:05:00" {
		t.Errorf("systemd: %s", c)
	}
	args := strings.Join(windowsTaskArgs(`C:\Program Files\omega\omega.exe`, "monthly", "quick", 4, 5), "|")
	if !strings.Contains(args, "/SC|MONTHLY|/D|1") || !strings.Contains(args, `/TR|"C:\Program Files\omega\omega.exe" quick --yes --no-tui|`) {
		t.Errorf("schtasks: %s", args)
	}
}
