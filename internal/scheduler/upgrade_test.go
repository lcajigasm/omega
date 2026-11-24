package scheduler

import (
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/runner"
)

const legacyXML = "\ufeff" + `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Actions Context="Author">
    <Exec>
      <Command>"C:\Users\me\scoop\shims\omega.exe"</Command>
      <Arguments>clean --yes --no-tui --no-ai</Arguments>
    </Exec>
  </Actions>
</Task>`

// A task written by an older omega has its command changed in place, once:
// FlagScheduled is added and the schedule is kept.
func TestUpgradeTaskAddsTheFlagOnce(t *testing.T) {
	ran := fake(t, "windows", func(cmd string) runner.Result {
		if strings.Contains(cmd, "/Query") {
			// The way schtasks writes UTF-16: a zero after every byte.
			return runner.Result{Output: strings.Join(strings.Split(legacyXML, ""), "\x00")}
		}
		return runner.Result{}
	})
	changed, err := UpgradeTask()
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	joined := strings.Join(*ran, "\n")
	if !strings.Contains(joined, "/Change") || !strings.Contains(joined, `"C:\Users\me\scoop\shims\omega.exe" clean --yes --no-tui --no-ai --scheduled`) {
		t.Fatalf("schtasks was not asked to change the command:\n%s", joined)
	}
	if strings.Contains(joined, "/Create") || strings.Contains(joined, "/Delete") {
		t.Fatalf("the task was recreated instead of changed:\n%s", joined)
	}
	n := len(*ran)
	if changed, _ := UpgradeTask(); changed || len(*ran) != n {
		t.Fatal("the task was checked again after it was brought up to date")
	}
}

func TestUpgradeTaskLeavesACurrentTask(t *testing.T) {
	current := strings.Replace(legacyXML, "--no-ai<", "--no-ai --scheduled<", 1)
	ran := fake(t, "windows", func(cmd string) runner.Result {
		if strings.Contains(cmd, "/Query") {
			return runner.Result{Output: current}
		}
		return runner.Result{}
	})
	if changed, err := UpgradeTask(); changed || err != nil {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if strings.Contains(strings.Join(*ran, "\n"), "/Change") {
		t.Fatal("a current task was changed")
	}
}

func TestUpgradeTaskOnlyOnWindows(t *testing.T) {
	ran := fake(t, "darwin", nil)
	if changed, _ := UpgradeTask(); changed || len(*ran) != 0 {
		t.Fatalf("UpgradeTask touched a Mac: %v", *ran)
	}
}

// The run an old task starts, before its command was changed, is still the
// scheduled job.
func TestIsLegacyRun(t *testing.T) {
	fake(t, "windows", nil)
	for _, c := range []struct {
		args []string
		want bool
	}{
		{[]string{"omega.exe", "clean", "--yes", "--no-tui", "--no-ai"}, true},
		{[]string{"omega.exe", "update", "--yes", "--no-tui", "--no-ai"}, true},
		{[]string{"omega.exe", "clean", "--yes"}, false},
		{[]string{"omega.exe", "clean", "--yes", "--no-tui", "--no-ai", "--scheduled"}, false},
		{[]string{"omega.exe", "nonsense", "--yes", "--no-tui", "--no-ai"}, false},
	} {
		if got := IsLegacyRun(c.args); got != c.want {
			t.Errorf("%v: %v, want %v", c.args, got, c.want)
		}
	}
	goos = "linux"
	if IsLegacyRun([]string{"omega", "clean", "--yes", "--no-tui", "--no-ai"}) {
		t.Error("only a Windows task lacks the environment variable")
	}
}
