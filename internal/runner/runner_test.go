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
	// Other names may be in the list: the credentials of this machine's own environment.
	if got := unsetWithheld("fish"); !strings.Contains(got, "set -e OMEGA_TEST_PROVIDER_KEY; ") || strings.Contains(got, "unset") {
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

// forget undoes what a test registered, so one test's credentials do not
// change what the next one's commands receive.
func forget(t *testing.T, names ...string) {
	t.Helper()
	t.Cleanup(func() {
		redactMu.Lock()
		defer redactMu.Unlock()
		for _, n := range names {
			delete(stripped, n)
		}
	})
}

// A variable whose name says it is a credential does not reach commands:
// printed in pieces, its value would get past the mask on the output. The
// few that tools need stay, masked.
func TestCredentialVariablesAreNotInherited(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	const secret = "s3cr3t-value-0123456789"
	for _, name := range []string{"OMEGA_T_DB_PASSWORD", "OMEGA_T_SIGNING_KEY", "OMEGA_T_PASS", "OMEGA_T_PWD", "OMEGA_T_AUTH", "OMEGA_T_DSN", "OMEGA_T_API_TOKEN"} {
		t.Setenv(name, secret)
		forget(t, name)
	}
	t.Setenv("GITHUB_TOKEN", "ghp_keptforbrewandgh0123456789")
	t.Setenv("OMEGA_T_AUTH_SOCK", "/tmp/agent.sock") // a path: where the secret is, not the secret
	t.Setenv("OMEGA_T_DATABASE_URL", "postgres://ana:hunter2pw@db.example.com/app")
	redactEnv()

	res := Run(context.Background(), `echo "a=[${OMEGA_T_DB_PASSWORD:0:6}] b=[$OMEGA_T_SIGNING_KEY] c=[$OMEGA_T_PASS$OMEGA_T_PWD$OMEGA_T_AUTH$OMEGA_T_DSN$OMEGA_T_API_TOKEN]"; `+
		`echo "sock=[$OMEGA_T_AUTH_SOCK]"; echo "url=$OMEGA_T_DATABASE_URL"; echo "gh=$GITHUB_TOKEN"; env | grep -c '^GITHUB_TOKEN='`, 15*time.Second, nil)
	for _, want := range []string{"a=[] b=[] c=[]", "sock=[/tmp/agent.sock]", "url=postgres://ana:[redacted]@db.example.com/app", "gh=[redacted]"} {
		if !strings.Contains(res.Output, want) {
			t.Errorf("want %q in %q", want, res.Output)
		}
	}
	for _, leak := range []string{"s3cr3t", "hunter2pw", "ghp_kept"} {
		if strings.Contains(res.Output, leak) {
			t.Errorf("%q reached the output: %q", leak, res.Output)
		}
	}
	if os.Getenv("OMEGA_T_DB_PASSWORD") != secret {
		t.Error("omega's own process must keep the variable")
	}
}

// What a command prints from a file omega never saw in an environment is
// masked by the shape of the line.
func TestScrubMasksAssignmentsAndURLs(t *testing.T) {
	for in, want := range map[string]string{
		`export OPENAI_API_KEY=sk-abc123`:                  `export OPENAI_API_KEY=[redacted]`,
		`export DB_PASS="p w d"`:                           `export DB_PASS=[redacted]`,
		`MYSQL_PWD=abcd1234`:                               `MYSQL_PWD=[redacted]`,
		`echo "OPENAI_API_KEY=[]" TOKEN_COUNT=5`:           `echo "OPENAI_API_KEY=[]" TOKEN_COUNT=5`,
		`SENTRY_DSN='https://k@sentry.io/1' # dsn`:         `SENTRY_DSN=[redacted] # dsn`,
		`remote = https://ana:ghp_abc@github.com/a/b.git`:  `remote = https://ana:[redacted]@github.com/a/b.git`,
		`psql postgres://u:pw@host/db --password=hunter22`: `psql postgres://u:[redacted]@host/db --password=[redacted]`,
		// Not credentials.
		`export PATH=/usr/bin:$PATH`:              `export PATH=/usr/bin:$PATH`,
		`SSH_AUTH_SOCK=/private/tmp/agent.sock`:   `SSH_AUTH_SOCK=/private/tmp/agent.sock`,
		`PWD=/Users/ana`:                          `PWD=/Users/ana`,
		`export EDITOR=vim LANG=en_US.UTF-8`:      `export EDITOR=vim LANG=en_US.UTF-8`,
		`https://example.com/a:b@c`:               `https://example.com/a:b@c`,
		`total 8 files, 10:30 ana@host`:           `total 8 files, 10:30 ana@host`,
		`KEYTIMEOUT=1`:                            `KEYTIMEOUT=1`,
		`user.signingkey=ABCDEF0123`:              `user.signingkey=ABCDEF0123`,
		`export AWS_SHARED_CREDENTIALS_FILE=~/.x`: `export AWS_SHARED_CREDENTIALS_FILE=~/.x`,
	} {
		if got := Scrub(in); got != want {
			t.Errorf("Scrub(%q) = %q, want %q", in, got, want)
		}
	}
}

// A key exported only by the login shell's startup files (the desktop app
// is started without them) is found, masked and taken out all the same.
func TestCredentialsFromStartupFilesAreFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("startup files of POSIX shells")
	}
	home := t.TempDir()
	rc := "export OMEGA_T_RC_SECRET=rc-only-secret-0123456789\nexport OMEGA_T_RC_PLAIN=rc-was-read\n"
	for _, name := range []string{".zshenv", ".zprofile", ".bash_profile", ".profile"} {
		if err := os.WriteFile(filepath.Join(home, name), []byte(rc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", home)
	forget(t, "OMEGA_T_RC_SECRET")
	for _, sh := range []string{"/bin/zsh", "/bin/bash"} {
		if _, err := os.Stat(sh); err != nil {
			continue
		}
		t.Setenv("SHELL", sh)
		vars := loginEnv()
		if !strings.Contains(strings.Join(vars, "\n"), "OMEGA_T_RC_PLAIN=rc-was-read") {
			t.Fatalf("%s: the login environment was not read: %d variables", sh, len(vars))
		}
		scanEnv(vars)
		res := Run(context.Background(), `echo "v=[$OMEGA_T_RC_SECRET] plain=[$OMEGA_T_RC_PLAIN]"; cat "$HOME/.profile"; echo "tail=${OMEGA_T_RC_SECRET:10}"`, 15*time.Second, nil)
		if strings.Contains(res.Output, "rc-only-secret") || !strings.Contains(res.Output, "v=[] plain=[rc-was-read]") ||
			!strings.Contains(res.Output, "export OMEGA_T_RC_SECRET=[redacted]") {
			t.Errorf("%s: %q", sh, res.Output)
		}
	}
}

// A command that leaves something running in the background has not failed.
func TestBackgroundChildIsNotAFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	start := time.Now()
	res := Run(context.Background(), `(sleep 20 &); echo started`, 30*time.Second, nil)
	if res.ExitCode != 0 || res.TimedOut || !strings.Contains(res.Output, "started") {
		t.Fatalf("exit %d timed out %v: %q", res.ExitCode, res.TimedOut, res.Output)
	}
	if !strings.Contains(res.Output, "still running") || strings.Contains(res.Output, "WaitDelay") {
		t.Errorf("the note about the background process is missing: %q", res.Output)
	}
	if time.Since(start) > 15*time.Second {
		t.Errorf("waited %s for the background process", time.Since(start))
	}
	// A command that fails keeps its own status.
	if res := Run(context.Background(), `(sleep 20 &); exit 3`, 30*time.Second, nil); res.ExitCode != 3 {
		t.Errorf("exit %d, want 3: %q", res.ExitCode, res.Output)
	}
}
