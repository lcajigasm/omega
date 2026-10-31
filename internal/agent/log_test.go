package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/config"
)

func TestRotateRunLogsKeepsNewest(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir := RunsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxRunLogs+5; i++ {
		name := fmt.Sprintf("2020-01-01_%06d.log", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(dir, "2026-10-31_120000.log")
	if err := os.WriteFile(current, []byte("current"), 0o644); err != nil {
		t.Fatal(err)
	}
	rotateRunLogs(dir, current, MaxRunLogs)

	logs, err := ListRunLogs()
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != MaxRunLogs {
		t.Fatalf("kept %d logs, want %d", len(logs), MaxRunLogs)
	}
	if logs[0].Name != "2026-10-31_120000.log" {
		t.Fatalf("newest = %s", logs[0].Name)
	}
	if _, err := os.Stat(filepath.Join(dir, "2020-01-01_000000.log")); !os.IsNotExist(err) {
		t.Fatal("oldest log was kept")
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(current); err != nil {
		t.Fatal(err)
	}
}

func TestOpenLogRotatesAndReadRunLog(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir := filepath.Join(config.StateDir(), "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxRunLogs+3; i++ {
		name := fmt.Sprintf("2020-01-01_%06d.log", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("old\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := &Agent{opts: Options{Model: "claude-test"}}
	a.openLog("anthropic")
	t.Cleanup(a.closeLog)

	logs, err := ListRunLogs()
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != MaxRunLogs {
		t.Fatalf("kept %d logs, want %d", len(logs), MaxRunLogs)
	}
	if _, b, err := ReadRunLog(""); err != nil || !strings.Contains(string(b), "anthropic") || !strings.Contains(string(b), "claude-test") {
		t.Fatalf("latest log: %v %q", err, b)
	}
	if _, b, err := ReadRunLog("2"); err != nil || string(b) != "old\n" {
		t.Fatalf("index 2: %v %q", err, b)
	}
	if _, _, err := ReadRunLog("../../etc/passwd"); err == nil {
		t.Fatal("path outside the log directory was read")
	}
	if _, _, err := ReadRunLog("99"); err == nil {
		t.Fatal("missing index was read")
	}
}
