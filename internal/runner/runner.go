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

	"github.com/lcajigasm/omega/internal/safety"
)

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Result is the outcome of a command.
type Result struct {
	ExitCode int
	Output   string // head + tail of combined stdout/stderr
	Duration time.Duration
	TimedOut bool
}

// maxLine is the longest line of output kept; the rest of it is dropped.
const maxLine = 64 * 1024

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
	redactMu.RLock()
	for n := range stripped {
		if envName.MatchString(n) && !withheld[n] {
			names = append(names, n)
		}
	}
	redactMu.RUnlock()
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

// secretName matches the names of environment variables that hold a
// credential: GITHUB_TOKEN, DB_PASSWORD, AWS_ACCESS_KEY_ID, PGPASS, SMTP_AUTH,
// SENTRY_DSN.
var secretName = regexp.MustCompile(`(?i)(token|secret|passw(or)?d|api_?key|credential|private_?key|(^|_)(key|pass|pwd|auth|dsn)(_|$))`)

// strongName are the words of secretName that mean a credential in any
// text, not only in the name of a variable.
var strongName = regexp.MustCompile(`(?i)(token|secret|passw(or)?d|api_?key|credential|private_?key)`)

var (
	// urlPassword is the password in scheme://user:password@host.
	urlPassword = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://[^/\s:@]*:)([^/\s@]+)(@)`)
	// assignment is NAME=value as env prints it and as a startup file
	// writes it (export NAME="value").
	assignment = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)=("[^"]*"|'[^']*'|[^\s"']+)`)
)

var (
	redactMu sync.RWMutex
	redacted []string
	// stripped are the variables commands do not get because their name
	// says they hold a credential. Guarded by redactMu.
	stripped = map[string]bool{}
	envOnce  sync.Once
)

// kept reports a credential that stays in the commands' environment
// because the tools omega runs need it. The safety policy refuses any
// command that names one.
func kept(name string) bool {
	for _, k := range safety.KeptSecretVars {
		if strings.EqualFold(k, name) {
			return true
		}
	}
	return false
}

// Redact makes every later command's output show "[redacted]" in place of
// the value: what a command prints goes to the model and to the run log.
// The sudo password is registered here when omega holds it for the run.
func Redact(values ...string) {
	redactMu.Lock()
	defer redactMu.Unlock()
	for _, v := range values {
		if len(v) < 4 {
			continue // too short to tell from ordinary text
		}
		known := false
		for _, r := range redacted {
			known = known || r == v
		}
		if !known {
			redacted = append(redacted, v)
		}
	}
	// Longest first, so a value that contains another is replaced whole.
	sort.Slice(redacted, func(i, j int) bool { return len(redacted[i]) > len(redacted[j]) })
}

// location reports a value that says where something is kept (a path),
// not what it is: SSH_AUTH_SOCK, PWD, AWS_SHARED_CREDENTIALS_FILE.
func location(value string) bool {
	return value == "" || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "~") ||
		(len(value) > 2 && value[1] == ':' && (value[2] == '\\' || value[2] == '/'))
}

// scanEnv reads NAME=value pairs. A value whose name says it is a
// credential is masked in every command's output, and the variable is taken
// out of the commands' environment: a command that prints it in pieces
// (${NAME:0:10}) would get past the mask. The few that tools need stay (see
// kept). The password of a URL is masked whatever the variable is called.
func scanEnv(vars []string) {
	for _, kv := range vars {
		name, value, _ := strings.Cut(kv, "=")
		if m := urlPassword.FindStringSubmatch(value); m != nil {
			Redact(m[2])
		}
		if !secretName.MatchString(name) || location(value) {
			continue
		}
		if len(value) >= 8 {
			Redact(value)
		}
		if !kept(name) {
			redactMu.Lock()
			stripped[name] = true
			redactMu.Unlock()
		}
	}
}

// redactEnv registers the credentials of omega's own environment.
func redactEnv() { scanEnv(os.Environ()) }

// loginEnvTimeout bounds the one look at the login shell's environment.
const loginEnvTimeout = 5 * time.Second

// loginEnv is the environment a login shell builds from the user's startup
// files. Commands run in such a shell, so a key exported in ~/.zshenv
// reaches them even when omega itself was started without it, as the
// desktop app is. It is nil when the shell cannot be asked: the run goes
// on without it.
func loginEnv() []string {
	sh := Shell()
	switch strings.ToLower(filepath.Base(sh)) {
	case "pwsh", "pwsh.exe", "powershell", "powershell.exe", "cmd", "cmd.exe":
		return nil // started without a profile: nothing is added to omega's environment
	}
	ctx, cancel := context.WithTimeout(context.Background(), loginEnvTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, sh, "-lc", "env -0 2>/dev/null || env")
	detach(cmd)
	cmd.Env = os.Environ()
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return nil
	}
	sep := "\n"
	if strings.Contains(string(out), "\x00") {
		sep = "\x00"
	}
	return strings.Split(string(out), sep)
}

// scrub replaces the registered values in a line of output, the value of
// any NAME=value whose name says it is a credential, and the password of a
// URL. The last two catch what omega never saw in an environment: a key
// written in a file the command prints.
func scrub(line string) string {
	redactMu.RLock()
	for _, v := range redacted {
		if strings.Contains(line, v) {
			line = strings.ReplaceAll(line, v, "[redacted]")
		}
	}
	redactMu.RUnlock()
	if strings.Contains(line, "://") {
		line = urlPassword.ReplaceAllString(line, "${1}[redacted]${3}")
	}
	if !strings.Contains(line, "=") {
		return line
	}
	return assignment.ReplaceAllStringFunc(line, func(m string) string {
		name, value, _ := strings.Cut(m, "=")
		// In running text the short words (key=, auth=) are everywhere:
		// they only count in a name written like a variable, DB_PASS.
		if !strongName.MatchString(name) && !(secretName.MatchString(name) && strings.ToUpper(name) == name) {
			return m
		}
		// A path, a value already masked, or one too short to be a secret
		// (an empty "[]", a flag such as 1).
		if v := strings.Trim(value, `"'`); location(v) || len(v) < 4 || strings.Contains(v, "[redacted]") {
			return m
		}
		return name + "=[redacted]"
	})
}

// Scrub is the masking applied to command output, for text that reaches
// the model another way (a file read by the agent).
func Scrub(text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = scrub(l)
	}
	return strings.Join(lines, "\n")
}

// childEnv is omega's environment without the withheld variables and
// without the ones that look like credentials.
func childEnv() []string {
	redactMu.RLock()
	defer redactMu.RUnlock()
	var out []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if !withheld[name] && !stripped[name] {
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
	envOnce.Do(func() {
		redactEnv()
		scanEnv(loginEnv())
	})
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
		// A line longer than the buffer is cut, not fatal: bufio.Scanner
		// stops for good on one, and everything the command printed after
		// it (and before it, in a single write) would be lost.
		rd := bufio.NewReaderSize(pr, maxLine)
		emit := func(raw []byte, cut bool) {
			line := scrub(stripANSI(string(raw)))
			if cut {
				line += " [omega: line cut]"
			}
			if strings.TrimSpace(line) == "" {
				return
			}
			buf.add(line + "\n")
			if onLine != nil {
				onLine(line)
			}
		}
		var line []byte
		cut := false
		for {
			c, err := rd.ReadByte()
			if err != nil {
				if len(line) > 0 {
					emit(line, cut)
				}
				break
			}
			// Progress bars use \r; treat it as a line break.
			if c == '\n' || c == '\r' {
				emit(line, cut)
				line, cut = line[:0], false
				continue
			}
			if len(line) < maxLine {
				line = append(line, c)
			} else {
				cut = true
			}
		}
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
	case errors.Is(err, exec.ErrWaitDelay):
		// The command itself ended. Something it left in the background
		// still holds its output open: that is not a failure.
		if cmd.ProcessState != nil {
			res.ExitCode = cmd.ProcessState.ExitCode()
		}
		res.Output += "\n[omega: the command finished; a process it started in the background is still running]"
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
