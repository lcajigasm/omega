package scheduler

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// legacyFlags are the flags a task written before FlagScheduled existed
// carries. A Windows task cannot set OMEGA_SCHEDULED, so such a task's runs
// looked like a person's: the trash step was not left out.
const legacyFlags = "--yes --no-tui --no-ai"

// windowsTask is the part of `schtasks /Query /XML` that says what runs.
// The XML is not translated, unlike the LIST output.
type windowsTask struct {
	Command   string `xml:"Actions>Exec>Command"`
	Arguments string `xml:"Actions>Exec>Arguments"`
}

// queryWindowsTask returns what the installed task runs. ok is false when
// there is no task.
func queryWindowsTask(ctx context.Context) (t windowsTask, ok bool) {
	res := run(ctx, fmt.Sprintf(`schtasks /Query /TN "%s" /XML`, windowsTaskName), 5*time.Second)
	if res.ExitCode != 0 {
		return t, false
	}
	// schtasks writes UTF-16 on some systems; the decoder is told any
	// charset is fine, and the elements are plain ASCII either way.
	text := strings.ReplaceAll(res.Output, "\x00", "")
	text = strings.TrimLeft(text, "\ufeff\xff\xfe")
	dec := xml.NewDecoder(strings.NewReader(text))
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	var doc struct {
		windowsTask
	}
	if err := dec.Decode(&doc); err != nil {
		return t, false
	}
	return doc.windowsTask, true
}

// legacyArgs reports whether a task's arguments are those of a task written
// before FlagScheduled, and returns the task word they start with.
func legacyArgs(args string) (task string, legacy bool) {
	f := strings.Fields(args)
	if len(f) == 0 || strings.Contains(args, FlagScheduled) {
		return "", false
	}
	if strings.Join(f[1:], " ") != legacyFlags {
		return "", false
	}
	return f[0], true
}

// IsLegacyRun reports whether this process was started by a Windows task
// written before FlagScheduled: its arguments are exactly that task's. Such
// a run is the scheduled job, and is treated as one.
func IsLegacyRun(args []string) bool {
	if goos != "windows" || len(args) < 2 {
		return false
	}
	task, ok := legacyArgs(strings.Join(args[1:], " "))
	return ok && Validate("weekly", task, 0, 0) == nil
}

// upgradeMark is written once the installed task has been checked, so the
// check (a schtasks call) happens once per machine, not on every run.
func upgradeMark() string { return filepath.Join(LogDir(), ".task-upgraded") }

// UpgradeTask brings a Windows task written by an older omega up to date:
// it adds FlagScheduled to what the task runs, keeping its schedule
// (schtasks /Change /TR). Nothing happens elsewhere, when there is no task,
// or once the task has been checked. It returns whether the task changed.
func UpgradeTask() (bool, error) {
	if goos != "windows" {
		return false, nil
	}
	if _, err := os.Stat(upgradeMark()); err == nil {
		return false, nil
	}
	ctx := context.Background()
	// No task, or one this omega wrote: either way there is nothing to
	// bring up to date now or later (a task installed from now on already
	// carries the flag), and the mark saves the check on every run.
	t, ok := queryWindowsTask(ctx)
	changed := false
	if _, legacy := legacyArgs(t.Arguments); ok && legacy {
		tr := fmt.Sprintf(`"%s" %s %s`, strings.Trim(t.Command, `"`), strings.TrimSpace(t.Arguments), FlagScheduled)
		if out, err := runArgs("schtasks", "/Change", "/TN", windowsTaskName, "/TR", tr); err != nil {
			return false, fmt.Errorf("schtasks /Change failed: %s", strings.TrimSpace(string(out)))
		}
		changed = true
	}
	if err := os.MkdirAll(LogDir(), 0o755); err == nil {
		_ = os.WriteFile(upgradeMark(), []byte(time.Now().Format(time.RFC3339)+"\n"), 0o644)
	}
	return changed, nil
}
