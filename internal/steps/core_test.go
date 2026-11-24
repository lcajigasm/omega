package steps

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// C4: when every outdated cask is one left out, grep keeps no line; under
// pipefail that must not fail the step.
func TestBrewDoesNotFailWhenNothingIsLeft(t *testing.T) {
	type shell struct{ path, pipefail string }
	var list []shell
	if p, err := exec.LookPath("bash"); err == nil {
		list = append(list, shell{p, "set -o pipefail; "})
	}
	if p, err := exec.LookPath("zsh"); err == nil {
		list = append(list, shell{p, "setopt pipefail; "})
	}
	if len(list) == 0 {
		t.Skip("no bash or zsh")
	}
	for _, sh := range list {
		// The fragment alone, with empty input.
		out, code := run(t, sh.path, sh.pipefail+`gone=x; printf '' | { grep -vxF "$gone" || true; } | xargs -r echo upgrade`, t.TempDir())
		if code != 0 {
			t.Errorf("%s: fragment exit %d: %s", sh.path, code, out)
		}
		// The whole steps, with only the gone cask outdated.
		for name, cmd := range map[string]string{"brew": brewUpgrade, "brew-greedy": brewGreedy} {
			bin, home, log := caskroom(t)
			room := filepath.Join(home, "Caskroom")
			os.WriteFile(filepath.Join(bin, "brew"), []byte("#!/bin/sh\necho \"brew $*\" >> \""+log+"\"\ncase \"$1\" in\n  --caskroom) echo \""+room+"\" ;;\n  outdated) echo gone ;;\nesac\n"), 0o755)
			out, code := run(t, sh.path, sh.pipefail+cmd, bin, "HOME="+home, "OMEGA_MIN_FREE_GB=0")
			calls, _ := os.ReadFile(log)
			if code != 0 || strings.Contains(string(calls), "upgrade --cask gone") {
				t.Errorf("%s %s: exit %d, %q, brew was asked:\n%s", sh.path, name, code, out, calls)
			}
		}
	}
	for _, cmd := range []string{brewUpgrade, brewGreedy} {
		if strings.Contains(cmd, `| grep -vxF "$gone" |`) {
			t.Errorf("a bare grep in the pipeline: %s", cmd)
		}
	}
}

// A9: mole-purge removes the user's project folders: it always asks.
// Scheduled updates keep running: brew and macos do not.
func TestAlwaysConfirmSteps(t *testing.T) {
	mac := ids(For("darwin"))
	if !mac["mole-purge"].AlwaysConfirm {
		t.Error("mole-purge must always ask")
	}
	for _, id := range []string{"brew", "brew-greedy", "macos", "mole-purge-preview"} {
		if mac[id].AlwaysConfirm {
			t.Errorf("%s always asks: a scheduled run would skip it", id)
		}
	}
}
