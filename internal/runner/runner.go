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
	"strings"
	"sync"
	"syscall"
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

// Shell returns the user's login shell, defaulting to zsh.
func Shell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/zsh"
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
	parts := strings.Split(os.Getenv("PATH"), ":")
	have := map[string]bool{}
	for _, p := range parts {
		have[p] = true
	}
	for _, e := range extra {
		if !have[e] {
			parts = append(parts, e)
		}
	}
	os.Setenv("PATH", strings.Join(parts, ":"))
}

// Has reports whether a binary is on PATH.
func Has(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

var (
	extraEnv   []string
	pathPrefix string
)

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
	script := command
	if pathPrefix != "" {
		// Login shells rebuild PATH (path_helper), so prepend after they start.
		script = "export PATH=" + shellQuote(pathPrefix) + ":\"$PATH\"; " + command
	}
	cmd := exec.CommandContext(ctx, Shell(), "-lc", script)
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// Kill the whole session on timeout or cancel, not just the shell.
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.Env = append(os.Environ(),
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
		c.head.WriteString(s)
		return
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
