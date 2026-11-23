package assist

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// withTools is fakeHome with some commands installed.
func withTools(t *testing.T, bins ...string) (home string) {
	home = fakeHome(t)
	prev := newEnv
	newEnv = func() *env {
		e := prev()
		e.look = func(c string) (string, error) {
			for _, b := range bins {
				if b == c {
					p := filepath.Join(home, "bin", c)
					write(t, p, "#!/bin/sh\necho 1.0\n")
					os.Chmod(p, 0o755)
					return p, nil
				}
			}
			return "", exec.ErrNotFound
		}
		return e
	}
	return home
}

// G3, D8: topgrade's checks find omega's CI image and pre_sudo/sudo_loop
// in any of its files, and fix them in the file where they count.
func TestTopgradeChecks(t *testing.T) {
	home := withTools(t, "topgrade")
	main := filepath.Join(home, ".config", "topgrade.toml")
	write(t, main, "[misc]\nsudo_loop = true\nassume_yes = true\n\n[containers]\nignored_containers = [\"mine\"]\n")
	write(t, filepath.Join(home, ".config", "topgrade.d", "omega.toml"), "[misc]\npre_sudo = true\n")

	r := Scan()
	ci, ok := r.Check("topgrade.omega-ci")
	if !ok || ci.OK {
		t.Fatalf("omega-ci: %+v", ci)
	}
	if _, err := Apply("topgrade.omega-ci", ""); err != nil {
		t.Fatal(err)
	}
	if got := read(t, main); !strings.Contains(got, `ignored_containers = ["mine", "omega:ci*"]`) {
		t.Fatalf("the list that counts keeps the user's entries:\n%s", got)
	}

	sudo, _ := Scan().Check("topgrade.pre-sudo")
	if sudo.OK || !strings.Contains(sudo.Current, "sudo_loop") || !strings.Contains(sudo.Current, "pre_sudo") {
		t.Fatalf("pre-sudo: %+v", sudo)
	}
	if _, err := Apply("topgrade.pre-sudo", ""); err != nil {
		t.Fatal(err)
	}
	r = Scan()
	if c, _ := r.Check("topgrade.pre-sudo"); !c.OK {
		t.Fatalf("still on: %+v", c)
	}
	if c, _ := r.Check("topgrade.omega-ci"); !c.OK {
		t.Fatalf("omega-ci not kept: %+v", c)
	}
	if !strings.Contains(read(t, main), "assume_yes = true") {
		t.Fatal("other settings must stay")
	}
}

// G6: Mole's whitelist gets Chrome's model, after Mole's own defaults when
// the file is new; without the defaults nothing is written.
func TestMoleKeepsChromesModel(t *testing.T) {
	home := withTools(t, "mo")
	os.MkdirAll(filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "OptGuideOnDeviceModel"), 0o755)
	old := moleDefaults
	t.Cleanup(func() { moleDefaults = old })
	moleDefaults = func(string) ([]string, error) { return []string{"$HOME/.gradle/caches/*"}, nil }

	c, ok := Scan().Check("mole.chrome-model")
	if !ok || c.OK {
		t.Fatalf("%+v", c)
	}
	if _, err := Apply("mole.chrome-model", ""); err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(home, ".config", "mole", "whitelist"))
	if !strings.Contains(got, "$HOME/.gradle/caches/*") || !strings.Contains(got, chromeModel) {
		t.Fatalf("whitelist:\n%s", got)
	}
	if c, _ := Scan().Check("mole.chrome-model"); !c.OK {
		t.Fatal("kept now")
	}

	os.Remove(filepath.Join(home, ".config", "mole", "whitelist"))
	moleDefaults = func(string) ([]string, error) { return nil, os.ErrNotExist }
	if _, err := Apply("mole.chrome-model", ""); err == nil {
		t.Fatal("a whitelist without Mole's defaults must not be written")
	}
}

// Mole's defaults are read from its own base.sh.
func TestMoleDefaultsFromBaseSh(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "lib", "core", "base.sh"), "x=1\ndeclare -a DEFAULT_WHITELIST_PATTERNS=(\n    \"$HOME/Library/Caches/ms-playwright*\"\n    # note\n    \"$FINDER_METADATA_SENTINEL\"\n)\n")
	write(t, filepath.Join(dir, "mole"), "#!/bin/sh\n")
	got, err := moleDefaults(filepath.Join(dir, "mole"))
	if err != nil || len(got) != 1 || got[0] != "$HOME/Library/Caches/ms-playwright*" {
		t.Fatalf("%v %v", got, err)
	}
}
