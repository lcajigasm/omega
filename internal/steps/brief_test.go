package steps

import (
	"strings"
	"testing"
)

func TestBriefDropsHelpers(t *testing.T) {
	got := Brief(brewUpgrade)
	if strings.Contains(got, "omega_space") || strings.Contains(got, "() {") {
		t.Fatalf("helper left in: %q", got)
	}
	if !strings.HasPrefix(got, "brew update") {
		t.Fatalf("Brief(brewUpgrade) = %q, want it to start with brew update", got)
	}
	if got := Brief(withSpace("npm", "npm update -g")); got != "npm update -g" {
		t.Fatalf("Brief = %q", got)
	}
	for _, goos := range []string{"darwin", "linux"} {
		if got := Brief(topgradeCommand(goos, false)); strings.Contains(got, "omega_tg_sudo()") || strings.Contains(got, "awk -v min") {
			t.Fatalf("topgrade on %s: %q", goos, got)
		}
	}
}

func TestBriefKeepsPlainCommands(t *testing.T) {
	for _, c := range []string{"", "brew cleanup --prune=all", `echo "omega_x() {"`, "omega_x() { broken"} {
		if got := Brief(c); got != c {
			t.Errorf("Brief(%q) = %q", c, got)
		}
	}
}
