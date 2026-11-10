// Package runner executes shell commands non-interactively, streaming their
// output line by line and keeping a bounded tail for the model to read.
package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Result is the outcome of a command.
type Result struct {
	ExitCode int
	Output   string // head + tail of combined stdout/stderr
	Duration time.Duration
	TimedOut bool
}

const (
	headBytes = 2000
	tailBytes = 8000
)

// Seams the tests replace.
var (
	goos     = runtime.GOOS
	lookPath = exec.LookPath
)

// Shell returns the shell omega runs commands with. On macOS and Linux it
// prefers SHELL, then zsh, bash and sh. On Windows it is always PowerShell
// (PowerShell 7 when installed, else the Windows PowerShell every Windows
// has): the steps and the safety policy are written for one dialect, and a
// POSIX shell that happens to be installed (Git Bash) does not see the
// Windows package managers the way PowerShell does.
func Shell() string {
	if goos == "windows" {
		for _, name := range []string{"pwsh", "powershell"} {
			if p, err := lookPath(name); err == nil {
				return p
			}
		}
		return "cmd.exe"
	}
	candidates := []string{os.Getenv("SHELL"), "/bin/zsh", "/usr/bin/zsh", "/bin/bash", "/usr/bin/bash", "/bin/sh"}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return "/bin/sh"
}

// psPrelude makes PowerShell behave when nobody is watching: output in
// UTF-8 (Windows PowerShell writes the console code page into a pipe), and
// no progress bars, which are drawn with cursor movements.
const psPrelude = "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; $ProgressPreference = 'SilentlyContinue'; "

// psExit gives the script the exit status of its last command. Left alone,
// `powershell -Command` answers 1 whenever the last statement set $? to
// false, which a cmdlet does for any error it merely reports (no events
// found, one locked file skipped), and it loses a program's own code. So: a
// program that failed keeps its code, and a cmdlet that reported an error
// and went on is not a failed command; its message is in the output. An
// error that stops the script never reaches this line: PowerShell exits 1.
const psExit = "\nif ($?) { exit 0 }; if ($LASTEXITCODE) { exit $LASTEXITCODE }; exit 0"

// shellInvocation builds the argv that runs script. POSIX shells are login
// shells so the user's PATH is present; the withheld variables are unset
// after the startup files have run. A sudo bridge path is prepended
// after the shell starts, because login shells rebuild PATH.
func shellInvocation(script string) (string, []string) {
	sh := Shell()
	base := strings.ToLower(filepath.Base(sh))
	switch base {
	case "pwsh", "pwsh.exe", "powershell", "powershell.exe":
		return sh, []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", psPrelude + script + psExit}
	case "cmd", "cmd.exe":
		return sh, []string{"/d", "/c", script}
	default:
		if pathPrefix != "" {
			script = "export PATH=" + shellQuote(pathPrefix) + ":\"$PATH\"; " + script
		}
		return sh, []string{"-lc", unsetWithheld(base) + script}
	}
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// unsetWithheld is what runs before a command in a POSIX login shell to
// drop the withheld variables again. Taking them out of the environment
// omega passes down is not enough: a login shell reads the user's startup
// files first, and that is where most people export an API key. PowerShell
// runs with -NoProfile and cmd with /d, so nothing sets them again there.
func unsetWithheld(shell string) string {
	var names []string
	for n := range withheld {
		if envName.MatchString(n) {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	if shell == "fish" {
		return "set -e " + strings.Join(names, "; set -e ") + "; "
	}
	return "unset " + strings.Join(names, " ") + "; "
}

// ExtendPath adds the usual package-manager bin directories to PATH so tools
// are found even when omega is launched from a minimal environment.
func ExtendPath() {
	home, _ := os.UserHomeDir()
	extra := []string{
		"/opt/homebrew/bin", "/opt/homebrew/sbin", "/usr/local/bin",
		filepath.Join(home, ".cargo/bin"), filepath.Join(home, ".local/bin"),
		filepath.Join(home, ".bun/bin"), filepath.Join(home, "go/bin"),
	}
	if goos == "windows" {
		// Scoop's shims, winget's links, Chocolatey, and the per-user tools.
		local, data := os.Getenv("LOCALAPPDATA"), os.Getenv("ProgramData")
		extra = []string{
			filepath.Join(home, "scoop", "shims"), filepath.Join(home, ".cargo", "bin"),
			filepath.Join(home, "go", "bin"), filepath.Join(home, ".bun", "bin"), filepath.Join(home, ".local", "bin"),
		}
		if local != "" {
			extra = append(extra, filepath.Join(local, "Microsoft", "WinGet", "Links"), filepath.Join(local, "Microsoft", "WindowsApps"))
		}
		if data != "" {
			extra = append(extra, filepath.Join(data, "chocolatey", "bin"))
		}
	}
	if runtime.GOOS == "linux" {
		// Homebrew on Linux, snaps and Flatpak's exported commands.
		extra = append(extra, "/home/linuxbrew/.linuxbrew/bin", "/home/linuxbrew/.linuxbrew/sbin",
			filepath.Join(home, ".linuxbrew/bin"), "/snap/bin", "/var/lib/flatpak/exports/bin")
	}
	sep := string(os.PathListSeparator)
	parts := strings.Split(os.Getenv("PATH"), sep)
	have := map[string]bool{}
	for _, p := range parts {
		have[p] = true
	}
	for _, e := range extra {
		if !have[e] {
			parts = append(parts, e)
		}
	}
	os.Setenv("PATH", strings.Join(parts, sep))
}

// Has reports whether a binary is on PATH.
func Has(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

var (
	extraEnv   []string
	pathPrefix string
	withheld   = map[string]bool{}
)

// Withhold names environment variables that commands must not inherit:
// the model providers' API keys. A command's output goes to the model, and
// "echo $ANTHROPIC_API_KEY" would otherwise hand it the key.
func Withhold(names ...string) {
	for _, n := range names {
		if n != "" {
			withheld[n] = true
		}
	}
}

// childEnv is omega's environment without the withheld variables.
func childEnv() []string {
	var out []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if !withheld[name] {
			out = append(out, kv)
		}
	}
	return out
}

// Configure adds environment variables to every command and a directory
// that goes first on their PATH (after the login shell has built it).
func Configure(env []string, prefix string) {
	extraEnv, pathPrefix = env, prefix
}

// Run executes command through the login shell with stdin closed and in its
// own session, without a controlling terminal: tools that would prompt fail
// fast (or go through omega's sudo bridge) instead of hanging or writing over
// the interface. onLine receives each line of output.
func Run(ctx context.Context, command string, timeout time.Duration, onLine func(string)) Result {
	start := time.Now()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	name, args := shellInvocation(command)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = nil
	// Own session, and kill the whole group on timeout or cancel.
	detach(cmd)
	cmd.Env = append(childEnv(),
		"NONINTERACTIVE=1",
		"HOMEBREW_NO_ENV_HINTS=1",
		"HOMEBREW_NO_AUTO_UPDATE=1",
		"HOMEBREW_COLOR=0",
		"NO_COLOR=1",
		"TERM=dumb",
		"PAGER=cat",
		"GIT_TERMINAL_PROMPT=0",
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	cmd.WaitDelay = 5 * time.Second

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	buf := &capture{}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		// Progress bars use \r; treat it as a line break.
		sc.Split(scanLinesCR)
		for sc.Scan() {
			line := stripANSI(sc.Text())
			if strings.TrimSpace(line) == "" {
				continue
			}
			buf.add(line + "\n")
			if onLine != nil {
				onLine(line)
			}
		}
		io.Copy(io.Discard, pr)
	}()

	err := cmd.Start()
	if err == nil {
		err = cmd.Wait()
	}
	pw.Close()
	wg.Wait()

	res := Result{Output: buf.String(), Duration: time.Since(start)}
	if ctx.Err() == context.DeadlineExceeded {
		res.TimedOut = true
		res.ExitCode = -1
		res.Output += fmt.Sprintf("\n[omega: timed out after %s]", timeout)
		return res
	}
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		res.ExitCode = ee.ExitCode()
	default:
		res.ExitCode = -1
		res.Output += "\n[omega: " + err.Error() + "]"
	}
	return res
}

// capture keeps the first headBytes and the last tailBytes of output.
type capture struct {
	head    strings.Builder
	tail    []byte
	dropped int
}

func (c *capture) add(s string) {
	if c.head.Len() < headBytes {
		room := headBytes - c.head.Len()
		if len(s) <= room {
			c.head.WriteString(s)
			return
		}
		// One line can be up to the scanner limit (1 MB). Keep the budget:
		// the head stays at headBytes and the rest competes for the tail.
		c.head.WriteString(s[:room])
		s = s[room:]
	}
	c.tail = append(c.tail, s...)
	if len(c.tail) > tailBytes*2 {
		cut := len(c.tail) - tailBytes
		c.dropped += cut
		c.tail = append([]byte(nil), c.tail[cut:]...)
	}
}

func (c *capture) String() string {
	tail := c.tail
	dropped := c.dropped
	if len(tail) > tailBytes {
		dropped += len(tail) - tailBytes
		tail = tail[len(tail)-tailBytes:]
	}
	if dropped == 0 {
		return c.head.String() + string(tail)
	}
	return fmt.Sprintf("%s\n[… %d bytes omitted …]\n%s", c.head.String(), dropped, tail)
}

func scanLinesCR(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func stripANSI(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
