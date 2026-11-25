package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/runner"
)

// C5 of the fifth review: du's exit status for a folder it cannot read
// does not fail the scan.
func TestDiskUsageIgnoresUnreadableFolders(t *testing.T) {
	onPlatform(t, "darwin", devtoolsMac)
	s := runScript(t, Options{}, Deny, nil, func(string) runner.Result { return runner.Result{Output: "1G\t/tmp/x"} },
		&llm.Reply{Calls: []llm.ToolCall{call("1", "disk_usage", map[string]any{"path": "/tmp"})}})
	if len(s.ran) != 1 || !strings.Contains(s.ran[0], "2>/dev/null || true; } | sort -hr") {
		t.Fatalf("du's status must not reach the pipeline: %v", s.ran)
	}
}

// C6: a file written with write_file clears what the session measured.
func TestWriteFileClearsTheSessionCache(t *testing.T) {
	onPlatform(t, "darwin", devtoolsMac)
	dir := t.TempDir()
	target := filepath.Join(dir, "topgrade.toml")
	du := &llm.Reply{Calls: []llm.ToolCall{call("1", "disk_usage", map[string]any{"path": dir})}}
	write := &llm.Reply{Calls: []llm.ToolCall{call("2", "write_file", map[string]any{"path": target, "content": "x = 1\n", "reason": "r"})}}
	s := runScript(t, Options{AutoYes: true}, Approve, nil, nil, du, write, du)
	n := 0
	for _, c := range s.ran {
		if strings.Contains(c, "du -xhd") {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("after write_file the scan must run again: ran %v", s.ran)
	}
	if b, _ := os.ReadFile(target); string(b) != "x = 1\n" {
		t.Fatalf("written: %q", b)
	}
}

// C6: granting a macOS permission clears the cache before the tool runs again.
func TestRegainAccessForgetsTheCache(t *testing.T) {
	a, _, _, _ := accessFixture(t, true, []string{"ok"}, "settings", "retry")
	a.remember("run_step:x", "cmd", "old")
	reran := false
	a.regainAccess(context.Background(), shellCall("ls"), "SystemPolicyAllFiles", func() (string, error) {
		reran = true
		if a.cache.Len() != 0 {
			t.Error("the cache must be empty when the tool runs again")
		}
		return "ok", nil
	}, "", nil)
	if !reran {
		t.Fatal("the tool was not run again after the permission was granted")
	}
}
