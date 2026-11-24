package config

import (
	"os"
	"strings"
	"testing"
)

// A6: a scheduled run, by the flag or by the variable, leaves the Trash and
// the Recycle Bin alone; another run does not.
func TestScheduledRunSkipsTrashOnEverySystem(t *testing.T) {
	t.Setenv("OMEGA_SCHEDULED", "")
	cfg := Default()
	for _, id := range []string{"trash", "recycle-bin"} {
		if cfg.Skipped(id) {
			t.Errorf("%s skipped outside a scheduled run", id)
		}
	}
	SetScheduled(true)
	for _, id := range []string{"trash", "recycle-bin"} {
		if !cfg.Skipped(id) {
			t.Errorf("--scheduled: %s not skipped", id)
		}
	}
	if cfg.Skipped("brew") {
		t.Error("--scheduled skipped brew")
	}
	SetScheduled(false)
	t.Setenv("OMEGA_SCHEDULED", "1")
	if !Scheduled() || !cfg.Skipped("recycle-bin") || !cfg.Skipped("trash") {
		t.Error("OMEGA_SCHEDULED=1 is a scheduled run")
	}
}

// D11: the effort reaches the API in lower case.
func TestEffortIsLowerCase(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	os.MkdirAll(Dir(), 0o755)
	os.WriteFile(Path(), []byte("effort = \"High\"\n"), 0o600)
	cfg, err := Load()
	if err != nil || cfg.Effort != "high" {
		t.Fatalf("effort %q, %v", cfg.Effort, err)
	}
	cfg.Effort = " XHigh"
	if err := cfg.Validate(); err != nil || cfg.Effort != "xhigh" {
		t.Errorf("validate: %q, %v", cfg.Effort, err)
	}
}

// D12: keys omega does not know are reported, not dropped in silence; the
// disk budgets must be sizes.
func TestUnknownKeysAndBudgets(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	os.MkdirAll(Dir(), 0o755)
	os.WriteFile(Path(), []byte("aproval = \"all\"\nprovider = \"openai\"\n[providers.openai]\nmodle = \"x\"\n[disk_budgets]\nxcode = \"8GB\"\n"), 0o600)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(UnknownKeys(), ","); got != "aproval,providers.openai.modle" {
		t.Errorf("unknown keys: %q", got)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("a good budget: %v", err)
	}
	cfg.DiskBudgets["claude"] = "lots"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "claude") {
		t.Errorf("a bad budget: %v", err)
	}
	os.WriteFile(Path(), []byte("approval = \"all\"\n"), 0o600)
	if _, err := Load(); err != nil || len(UnknownKeys()) != 0 {
		t.Errorf("a clean file: %q %v", UnknownKeys(), err)
	}
	for in, want := range map[string]int64{"500MB": 500 << 20, "8 GB": 8 << 30, "1.5gb": 3 << 29, "2": 2 << 30} {
		if n, err := ParseSize(in); err != nil || n != want {
			t.Errorf("ParseSize(%q) = %d, %v", in, n, err)
		}
	}
}
