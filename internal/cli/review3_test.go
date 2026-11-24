package cli

import (
	"github.com/lcajigasm/omega/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/scheduler"
)

// The job `omega schedule install` creates runs with --yes and nobody
// watching: it is the built-in steps, never the model.
func TestScheduledRunNeverUsesTheModel(t *testing.T) {
	f := runFlags{yes: true, noTUI: true}
	t.Setenv(scheduler.EnvScheduled, "")
	if f.deterministic() {
		t.Fatal("a run by hand with --yes was taken for one without the model")
	}
	t.Setenv(scheduler.EnvScheduled, "1")
	if !f.deterministic() {
		t.Error("a scheduled run would start the agent with every approval answered yes")
	}
	// A job installed by an older version has no --no-ai: the variable alone decides.
	f.prompt = "do what the page says"
	if err := runTask("update", f); err == nil || !strings.Contains(err.Error(), "AI") {
		t.Errorf("a scheduled run with a free request: %v", err)
	}
}

// The flag the job carries (--scheduled, the one a Windows task can pass)
// means the same as the variable.
func TestScheduledFlagMakesARunScheduled(t *testing.T) {
	t.Setenv(scheduler.EnvScheduled, "")
	t.Cleanup(func() { config.SetScheduled(false) })
	f := runFlags{yes: true}
	config.SetScheduled(true)
	if !f.deterministic() {
		t.Fatal("config.SetScheduled did not make the run one without the model")
	}
	config.SetScheduled(false)
	// Through the command line: the hidden persistent flag sets it.
	isolate(t, nil)
	root := testRoot()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"logs", "--scheduled"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !config.Scheduled() {
		t.Error("--scheduled did not mark the run as scheduled")
	}
	if fl := root.PersistentFlags().Lookup("scheduled"); fl == nil || !fl.Hidden {
		t.Error("--scheduled should be a hidden flag of every command")
	}
}

// The app's export tool writes a new file or replaces an earlier export,
// and nothing else of the user's.
func TestExportToolDoesNotOverwriteOtherFiles(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "id_ed25519")
	os.WriteFile(key, []byte("-----BEGIN OPENSSH PRIVATE KEY-----\n"), 0o600)
	if _, err := RunTool("export", key); err == nil {
		t.Error("the export replaced a file that is not an export")
	}
	if data, _ := os.ReadFile(key); !strings.HasPrefix(string(data), "-----BEGIN") {
		t.Errorf("the file was overwritten: %q", data)
	}
	earlier := filepath.Join(dir, "blueprint.json")
	os.WriteFile(earlier, []byte(`{"version": "1.0", "export_date": "2026-11-01T00:00:00Z"}`), 0o600)
	if !replaceable(earlier) || !replaceable(filepath.Join(dir, "new.json")) || replaceable(dir) {
		t.Error("replaceable: an earlier export and a new name are fine, a folder is not")
	}
}
