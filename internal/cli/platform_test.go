package cli

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/devtools"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/runner"
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

// An installer piped into a shell needs a typed yes every time, also when
// the tool was named on the command line, and never runs without a terminal.
func TestPipedInstallerAlwaysAsks(t *testing.T) {
	i18n.Init("en")
	old := devtools.Current
	devtools.Current = func() devtools.Platform { return devtools.Platform{OS: "linux", Pkg: "apt", Shell: "bash"} }
	t.Cleanup(func() { devtools.Current = old })
	mise, _ := devtools.Find("mise")
	jq, _ := devtools.Find("jq")
	if !strings.Contains(mise.Install, "| sh") || strings.Contains(jq.Install, "curl") {
		t.Fatalf("mise uses its installer, jq a package: %q %q", mise.Install, jq.Install)
	}
	asked := 0
	answer := func(reply string) func(string, string) string {
		return func(prompt, def string) string {
			asked++
			if !strings.Contains(prompt, "mise") || def != "n" {
				t.Errorf("prompt %q, default %q", prompt, def)
			}
			return reply
		}
	}
	if ok, err := allowInstaller(mise, true, answer("y")); !ok || err != nil || asked != 1 {
		t.Fatalf("yes runs it after asking: %v %v, asked %d", ok, err, asked)
	}
	if ok, err := allowInstaller(mise, true, answer("")); ok || err != nil {
		t.Fatalf("Enter means no: %v %v", ok, err)
	}
	if ok, err := allowInstaller(mise, true, answer("n")); ok || err != nil {
		t.Fatalf("no skips it: %v %v", ok, err)
	}
	asked = 0
	ok, err := allowInstaller(mise, false, answer("y"))
	if ok || err == nil || asked != 0 || !strings.Contains(err.Error(), "terminal") || !strings.Contains(err.Error(), mise.Install) {
		t.Fatalf("without a terminal it is refused, not asked: %v %v, asked %d", ok, err, asked)
	}
	if ok, err := allowInstaller(jq, false, answer("n")); !ok || err != nil || asked != 0 {
		t.Fatalf("a package install is not this card's business: %v %v, asked %d", ok, err, asked)
	}
}

// Every provider key variable is withheld from the commands omega runs, and
// omega itself still reads the key from its own environment.
func TestProviderKeysAreWithheldFromCommandsOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the command below is written for a POSIX shell")
	}
	withholdProviderKeys()
	t.Setenv("HOME", t.TempDir())
	var script []string
	for provider, name := range secrets.EnvVar {
		t.Setenv(name, "sk-leak-test-"+provider+"-0123456789")
		script = append(script, `echo "`+name+`=[$`+name+`]"`)
	}
	res := runner.Run(context.Background(), strings.Join(script, "; ")+"; env", 15*time.Second, nil)
	if strings.Contains(res.Output, "sk-leak-test-") {
		t.Fatalf("a provider key reached a command: %q", res.Output)
	}
	for provider, name := range secrets.EnvVar {
		if !strings.Contains(res.Output, name+"=[]") {
			t.Errorf("%s was not checked: %q", name, res.Output)
		}
		if key, src, err := secrets.Get(provider); err != nil || src != secrets.FromEnv || key != "sk-leak-test-"+provider+"-0123456789" {
			t.Errorf("omega must still read %s itself: %q %s %v", name, key, src, err)
		}
	}
}
