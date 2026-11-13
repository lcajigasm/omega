package runner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestShellExists(t *testing.T) {
	t.Setenv("SHELL", "/no/such/omega-shell")
	sh := Shell()
	if _, err := os.Stat(sh); err != nil && sh != "cmd.exe" {
		t.Fatalf("Shell() = %q, not a usable shell", sh)
	}
}

func TestExtendPathUsesListSeparator(t *testing.T) {
	t.Setenv("PATH", string(os.PathListSeparator)+"usr-bin-only")
	ExtendPath()
	got := os.Getenv("PATH")
	want := "/opt/homebrew/bin"
	if runtime.GOOS == "windows" {
		want = filepath.Join("scoop", "shims")
	}
	if !strings.Contains(got, want) {
		t.Fatalf("PATH missing %s: %s", want, got)
	}
	if !strings.Contains(got, string(os.PathListSeparator)) {
		t.Fatalf("PATH %q has no list separator", got)
	}
}

func TestCaptureBoundsAHugeLine(t *testing.T) {
	const raw = 200_000
	c := &capture{}
	c.add(strings.Repeat("x", raw) + "\n")
	got := c.String()
	const budget = headBytes + tailBytes + 64
	if len(got) > budget {
		t.Fatalf("kept %d bytes of a %d byte line, budget %d", len(got), raw, budget)
	}
	if !strings.Contains(got, "omitted") {
		t.Fatalf("huge line was not marked omitted: %d bytes kept", len(got))
	}
	t.Logf("a %d byte line is kept as %d bytes (head %d + tail %d)", raw, len(got), headBytes, tailBytes)
}

func TestRunEcho(t *testing.T) {
	res := Run(context.Background(), "echo omega-ok", 15*time.Second, nil)
	if res.ExitCode != 0 || !strings.Contains(res.Output, "omega-ok") {
		t.Fatalf("echo failed: exit %d output %q", res.ExitCode, res.Output)
	}
}

// The model providers' keys stay with omega: a command must not see them,
// because its output is sent to the model.
func TestWithheldVariablesDoNotReachCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the command below is written for a POSIX shell")
	}
	t.Setenv("OMEGA_TEST_PROVIDER_KEY", "sk-must-not-leak-0001")
	t.Setenv("OMEGA_TEST_OTHER", "visible")
	Withhold("OMEGA_TEST_PROVIDER_KEY")
	t.Cleanup(func() { delete(withheld, "OMEGA_TEST_PROVIDER_KEY") })
	res := Run(context.Background(), `echo "key=[$OMEGA_TEST_PROVIDER_KEY] other=[$OMEGA_TEST_OTHER]"; env | grep -c OMEGA_TEST_PROVIDER_KEY`, 10*time.Second, nil)
	if strings.Contains(res.Output, "sk-must-not-leak") || !strings.Contains(res.Output, "key=[]") || !strings.Contains(res.Output, "other=[visible]") {
		t.Fatalf("output: %q", res.Output)
	}
}

// Most people export an API key in a shell startup file. Commands run in a
// login shell, which reads those files after omega has taken the variable
// out of the environment: it must be gone when the command runs anyway.
func TestWithheldVariablesSetByStartupFilesDoNotReachCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("startup files of POSIX shells")
	}
	home := t.TempDir()
	rc := "export OMEGA_TEST_PROVIDER_KEY=sk-from-startup-file-0002\nexport OMEGA_TEST_FROM_RC=rc-was-read\n"
	for _, name := range []string{".zshenv", ".zprofile", ".zshrc", ".bash_profile", ".bashrc", ".profile"} {
		if err := os.WriteFile(filepath.Join(home, name), []byte(rc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", home)
	t.Setenv("OMEGA_TEST_PROVIDER_KEY", "sk-from-environment-0001")
	Withhold("OMEGA_TEST_PROVIDER_KEY")
	t.Cleanup(func() { delete(withheld, "OMEGA_TEST_PROVIDER_KEY") })

	for _, sh := range []string{"/bin/zsh", "/usr/bin/zsh", "/bin/bash", "/usr/bin/bash", "/bin/sh"} {
		if _, err := os.Stat(sh); err != nil {
			continue
		}
		t.Setenv("SHELL", sh)
		res := Run(context.Background(), `echo "key=[$OMEGA_TEST_PROVIDER_KEY] rc=[$OMEGA_TEST_FROM_RC]"; printenv OMEGA_TEST_PROVIDER_KEY; env | grep OMEGA_TEST_PROVIDER_KEY; true`, 15*time.Second, nil)
		if strings.Contains(res.Output, "sk-from-") || !strings.Contains(res.Output, "key=[]") {
			t.Errorf("%s: the key reached the command: %q", sh, res.Output)
		}
		// Without this the test would pass for a shell that read no file.
		if filepath.Base(sh) != "sh" && !strings.Contains(res.Output, "rc=[rc-was-read]") {
			t.Errorf("%s: the startup files were not read, so nothing was proved: %q", sh, res.Output)
		}
	}
	if os.Getenv("OMEGA_TEST_PROVIDER_KEY") != "sk-from-environment-0001" {
		t.Error("omega's own process must keep the variable: only its commands lose it")
	}
	if got := unsetWithheld("fish"); got != "set -e OMEGA_TEST_PROVIDER_KEY; " {
		t.Errorf("fish: %q", got)
	}
}

// On Windows commands run in PowerShell: output comes back as UTF-8, a
// program's exit code survives, and the provider keys are not inherited.
func TestPowerShellOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell is the shell on Windows only")
	}
	if base := strings.ToLower(filepath.Base(Shell())); !strings.HasPrefix(base, "pwsh") && !strings.HasPrefix(base, "powershell") {
		t.Fatalf("shell is %s", Shell())
	}
	res := Run(context.Background(), `Write-Output "hola ñ €"`, 60*time.Second, nil)
	if res.ExitCode != 0 || !strings.Contains(res.Output, "hola ñ €") {
		t.Fatalf("output %q exit %d", res.Output, res.ExitCode)
	}
	if res := Run(context.Background(), `cmd /c exit 7`, 60*time.Second, nil); res.ExitCode != 7 {
		t.Fatalf("a program's exit code is lost: %d %q", res.ExitCode, res.Output)
	}
	if res := Run(context.Background(), `Get-Item C:\does-not-exist-omega -ErrorAction Stop`, 60*time.Second, nil); res.ExitCode == 0 {
		t.Fatalf("a failed cmdlet reports success: %q", res.Output)
	}
	// A cmdlet that reports an error and carries on is not a failed command.
	if res := Run(context.Background(), `Get-WinEvent -FilterHashtable @{LogName='Application'; ProviderName='omega-no-such-provider'} -MaxEvents 1 -ErrorAction SilentlyContinue`, 60*time.Second, nil); res.ExitCode != 0 {
		t.Fatalf("a silenced, non-terminating error failed the command: %d %q", res.ExitCode, res.Output)
	}
	if res := Run(context.Background(), `Write-Output one; Write-Output two`, 60*time.Second, nil); res.ExitCode != 0 || !strings.Contains(res.Output, "two") {
		t.Fatalf("two statements: %d %q", res.ExitCode, res.Output)
	}
	t.Setenv("OMEGA_TEST_PROVIDER_KEY", "sk-must-not-leak-0003")
	Withhold("OMEGA_TEST_PROVIDER_KEY")
	t.Cleanup(func() { delete(withheld, "OMEGA_TEST_PROVIDER_KEY") })
	res = Run(context.Background(), `"key=[$env:OMEGA_TEST_PROVIDER_KEY]"; Get-ChildItem env: | Out-String`, 60*time.Second, nil)
	if strings.Contains(res.Output, "sk-must-not-leak") || !strings.Contains(res.Output, "key=[]") {
		t.Fatalf("the key reached the command: %q", res.Output)
	}
}

// A line longer than the buffer is cut; what came before and after it is
// still there.
func TestRunKeepsOutputAroundAHugeLine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	res := Run(context.Background(), `echo HEAD_MARKER; head -c 1100000 /dev/zero | tr '\0' a; echo; echo TAIL_MARKER`, 20*time.Second, nil)
	if res.ExitCode != 0 || !strings.Contains(res.Output, "HEAD_MARKER") || !strings.Contains(res.Output, "TAIL_MARKER") {
		t.Fatalf("exit %d, output of %d bytes lost its ends", res.ExitCode, len(res.Output))
	}
}

// A registered secret never appears in what a command printed.
func TestRunRedacts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	Redact("hunter2-sudo-password")
	var lines []string
	res := Run(context.Background(), `echo "pw=hunter2-sudo-password end"`, 10*time.Second, func(l string) { lines = append(lines, l) })
	if strings.Contains(res.Output, "hunter2") || !strings.Contains(res.Output, "pw=[redacted] end") {
		t.Errorf("output: %q", res.Output)
	}
	if strings.Contains(strings.Join(lines, ""), "hunter2") {
		t.Errorf("the live lines show the secret: %q", lines)
	}
}

// Credentials in omega's own environment are not shown by env.
func TestRunRedactsEnvironmentCredentials(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	t.Setenv("ZZ_FAKE_TOKEN", "tok-1234567890abcdef")
	redactEnv()
	res := Run(context.Background(), `printenv ZZ_FAKE_TOKEN`, 10*time.Second, nil)
	if strings.Contains(res.Output, "tok-1234567890abcdef") {
		t.Errorf("the token reached the output: %q", res.Output)
	}
}
