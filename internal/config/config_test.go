package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSaveRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := Default()
	cfg.Effort = "xhigh"
	cfg.Language = "es"
	cfg.SkipSteps = []string{"trash", "docker"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Effort != "xhigh" || got.Language != "es" || len(got.SkipSteps) != 2 || got.SkipSteps[0] != "trash" {
		t.Fatalf("round trip: %+v", got)
	}
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSaveKeepsFileWhenInvalid(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := Default()
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg.Approval = "sometimes"
	if err := cfg.Save(); err == nil {
		t.Fatal("invalid approval was saved")
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Approval != ApproveDangerous {
		t.Fatalf("approval = %q, file was replaced", got.Approval)
	}
}

func TestValidate(t *testing.T) {
	ok := Default()
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := []func(*Config){
		func(c *Config) { c.Approval = "yes" },
		func(c *Config) { c.Language = "it" },
		func(c *Config) { c.Timeout = "soon" },
		func(c *Config) { c.Timeout = "0s" },
		func(c *Config) { c.Effort = "ultra" },
		func(c *Config) { c.SkipSteps = []string{"not-a-step"} },
	}
	for _, mutate := range cases {
		cfg := Default()
		mutate(cfg)
		if err := cfg.Validate(); err == nil {
			t.Fatalf("Validate accepted %+v", cfg)
		}
	}
}

func TestLoadFillsEmptyFields(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "omega", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("provider = \"\"\napproval = \"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "anthropic" || cfg.Approval != ApproveDangerous || cfg.Timeout != "45m" || cfg.Language != "auto" {
		t.Fatalf("defaults not filled: %+v", cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

// A config.toml that is a symbolic link stays one: the file it points to is
// the one that changes.
func TestSaveKeepsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need a privilege on Windows")
	}
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	dotfiles := t.TempDir()
	target := filepath.Join(dotfiles, "omega.toml")
	if err := os.WriteFile(target, []byte("language = \"en\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, Path()); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Language = "es"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(Path()); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config.toml is no longer a symbolic link (%v)", err)
	}
	b, _ := os.ReadFile(target)
	if !strings.Contains(string(b), `language = "es"`) {
		t.Errorf("the file the link points to was not written: %s", b)
	}
	if left, _ := filepath.Glob(filepath.Join(dotfiles, ".config-*")); len(left) != 0 {
		t.Errorf("temporary files left behind: %v", left)
	}
}

// The Trash is where omega puts what it removes. A scheduled run, which
// nobody watches, must not empty it.
func TestScheduledRunKeepsTheTrash(t *testing.T) {
	c := &Config{}
	if c.Skipped("trash") {
		t.Fatal("trash is skipped outside a scheduled run")
	}
	t.Setenv("OMEGA_SCHEDULED", "1")
	if !c.Skipped("trash") {
		t.Error("a scheduled run would empty the Trash")
	}
	if c.Skipped("brew") {
		t.Error("a scheduled run skips other steps")
	}
}
