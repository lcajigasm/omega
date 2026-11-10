package cli

import (
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/secrets"
)

func onOS(t *testing.T, os string) {
	t.Helper()
	old := goos
	goos = os
	t.Cleanup(func() { goos = old })
}

// Off macOS, omega mac and every subcommand say so before touching anything.
func TestMacCommandsAreMacOnly(t *testing.T) {
	i18n.Init("en")
	onOS(t, "linux")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, args := range [][]string{{}, {"status"}, {"login"}, {"permissions"}, {"apps"}, {"undo"}, {"policy"}, {"op", "dock-remove", "x"}} {
		c := macCmd()
		c.SetArgs(args)
		c.SilenceUsage, c.SilenceErrors = true, true
		err := c.Execute()
		if err == nil || !strings.Contains(err.Error(), "only works on macOS") || !strings.Contains(err.Error(), "Linux") {
			t.Errorf("omega mac %v: %v", args, err)
		}
		want := strings.TrimSpace("omega mac " + strings.Join(args[:min(len(args), 1)], " "))
		if err != nil && !strings.Contains(err.Error(), want) {
			t.Errorf("the message names the command %q: %v", want, err)
		}
	}
}

func TestSudoIsOnlyAskedWhenACommandUsesIt(t *testing.T) {
	for cmd, want := range map[string]bool{
		"sudo apt-get update && sudo apt-get -y upgrade":             true,
		"docker info >/dev/null 2>&1 || sudo systemctl start docker": true,
		"pacman -Qdtq | xargs -r sudo pacman -Rns --noconfirm":       true,
		"brew install jq":                    false,
		"flatpak update -y --noninteractive": false,
		"echo pseudo-sudoers":                false,
		"curl -fsSL https://example.com/sudo/install.sh -o install.sh": false,
	} {
		if got := usesSudo.MatchString(cmd); got != want {
			t.Errorf("usesSudo(%q) = %v, want %v", cmd, got, want)
		}
	}
}

// With no key, doctor says where omega setup would put one.
func TestDoctorSaysWhereAKeyWouldGo(t *testing.T) {
	writeDoctorConfig(t, "provider = \"anthropic\"\n")
	stubKey(t, "", secrets.ErrNotFound)
	old := keyStore
	keyStore = func() secrets.Source { return secrets.FromFile }
	t.Cleanup(func() { keyStore = old })
	out, err := runDoctorOut(t)
	if err == nil {
		t.Fatal("a missing key must fail")
	}
	if !strings.Contains(out, "key: missing") || !strings.Contains(out, "key storage:") || !strings.Contains(out, secrets.FilePath()) {
		t.Fatalf("output:\n%s", out)
	}
}
