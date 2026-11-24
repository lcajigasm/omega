package runner

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// M4: the head and the tail of a capture never hold half a character.
func TestCaptureKeepsWholeCharacters(t *testing.T) {
	c := &capture{}
	c.add(strings.Repeat("ñ", headBytes) + "\n") // 2 bytes each, odd offsets
	c.add(strings.Repeat("é", tailBytes*3) + "x\n")
	out := c.String()
	if !utf8.ValidString(out) {
		t.Fatalf("the capture split a character")
	}
	c = &capture{}
	c.add("a" + strings.Repeat("ü", headBytes))
	if !utf8.ValidString(c.String()) {
		t.Fatalf("the head split a character")
	}
	if got := string(wholeRunes([]byte("abc\xe2\x82"))); got != "abc" {
		t.Errorf("wholeRunes = %q", got)
	}
	if got := string(wholeRunes([]byte("abc€"))); got != "abc€" {
		t.Errorf("wholeRunes = %q", got)
	}
}

// M2: one quoting helper for the shell.
func TestShellQuote(t *testing.T) {
	if got := ShellQuote("it's"); got != `'it'\''s'` {
		t.Errorf("ShellQuote = %s", got)
	}
}

// D8: a timeout sends SIGTERM first, so a command can let go of its locks.
func TestTimeoutTerminatesBeforeKilling(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signals")
	}
	// The login shell reads its profile before the trap is set: on a busy
	// machine that can take most of a second.
	res := Run(context.Background(), `trap 'echo got-term; exit 3' TERM; sleep 30 & wait`, 3*time.Second, nil)
	if !res.TimedOut || !strings.Contains(res.Output, "got-term") {
		t.Fatalf("the command was not told to stop: %+v", res)
	}
}

// D8: what ignores SIGTERM is killed after the grace period.
func TestTimeoutKillsWhatIgnoresTerm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signals")
	}
	old := killGrace
	killGrace = 300 * time.Millisecond
	defer func() { killGrace = old }()
	start := time.Now()
	res := Run(context.Background(), `trap '' TERM; sleep 30`, 300*time.Millisecond, nil)
	if !res.TimedOut {
		t.Fatalf("not timed out: %+v", res)
	}
	if d := time.Since(start); d > 4*time.Second {
		t.Fatalf("took %s: the group was not killed after the grace period", d)
	}
}
