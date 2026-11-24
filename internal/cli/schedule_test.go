package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/scheduler"
)

func TestNextRun(t *testing.T) {
	// Wednesday 2026-11-25 10:00.
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		interval string
		hour     int
		want     string
	}{
		{"daily", 3, "2026-11-26 03:00"},
		{"daily", 12, "2026-11-25 12:00"},
		{"weekly", 3, "2026-11-28 03:00"},
		{"monthly", 3, "2026-12-01 03:00"},
	} {
		if got := nextRun(c.interval, c.hour, 0, now).Format("2006-01-02 15:04"); got != c.want {
			t.Errorf("%s at %d: %s, want %s", c.interval, c.hour, got, c.want)
		}
	}
}

// The status reads the job file: task, interval, time, next run and the
// last exit code launchd recorded.
func TestDescribeScheduleMac(t *testing.T) {
	dir := t.TempDir()
	plist := filepath.Join(dir, "com.omega.maintenance.plist")
	os.WriteFile(plist, []byte(`<plist><dict><key>Label</key><string>com.omega.maintenance</string>
<key>ProgramArguments</key><array><string>/opt/omega</string><string>clean</string><string>--yes</string></array>
<key>StartCalendarInterval</key><dict><key>Weekday</key><integer>6</integer><key>Hour</key><integer>4</integer><key>Minute</key><integer>30</integer></dict>
</dict></plist>`), 0o644)
	old := scheduleRun
	t.Cleanup(func() { scheduleRun = old })
	scheduleRun = func(string) runner.Result { return runner.Result{Output: "state = waiting\n\tlast exit code = 2\n"} }

	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.Local)
	v := describeSchedule(&scheduler.ScheduleStatus{Installed: true, Platform: "darwin", Target: plist}, "darwin", now)
	if v.Task != "clean" || v.Interval != "weekly" || v.Time != "04:30" || v.Next.Format("2006-01-02 15:04") != "2026-11-28 04:30" {
		t.Errorf("view: %+v", v)
	}
	if v.LastExit == nil || *v.LastExit != 2 {
		t.Errorf("last exit: %v", v.LastExit)
	}
}

// The command a scheduled custom task runs is one omega understands: the
// flags every scheduled task gets parse on `omega custom`.
func TestScheduledCustomFlagsParse(t *testing.T) {
	c, _, err := testRoot().Find([]string{"custom"})
	if err != nil || c.Name() != "custom" {
		t.Fatalf("custom: %v", err)
	}
	if err := c.ParseFlags([]string{"--yes", "--no-tui", "--no-ai", scheduler.FlagScheduled}); err != nil {
		t.Errorf("custom flags: %v", err)
	}
	if err := scheduler.Validate("weekly", "custom", 3, 0); err != nil {
		t.Errorf("validate: %v", err)
	}
}
