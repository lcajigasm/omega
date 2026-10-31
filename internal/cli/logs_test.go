package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
)

func TestLogsListAndView(t *testing.T) {
	i18n.Init("en")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir := agent.RunsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2024-01-01_000000.log"), []byte("alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2024-06-01_000000.log"), []byte("beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	c := logsCmd()
	c.SetOut(&buf)
	c.SetErr(&buf)
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	newer := strings.Index(out, "2024-06-01_000000.log")
	older := strings.Index(out, "2024-01-01_000000.log")
	if newer < 0 || older < 0 || newer > older {
		t.Fatalf("newest should be listed first:\n%s", out)
	}
	if !strings.Contains(out, dir) || !strings.Contains(out, "keeping the newest") {
		t.Fatalf("list header:\n%s", out)
	}

	buf.Reset()
	c = logsCmd()
	c.SetOut(&buf)
	c.SetArgs([]string{"view"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "beta\n" {
		t.Fatalf("view latest: %q", buf.String())
	}

	buf.Reset()
	c = logsCmd()
	c.SetOut(&buf)
	c.SetArgs([]string{"view", "2"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "alpha\n" {
		t.Fatalf("view 2: %q", buf.String())
	}

	buf.Reset()
	c = logsCmd()
	c.SetOut(&buf)
	c.SetArgs([]string{"view", "2024-01-01_000000"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "alpha\n" {
		t.Fatalf("view by name: %q", buf.String())
	}
}

func TestLogsEmpty(t *testing.T) {
	i18n.Init("en")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	var buf bytes.Buffer
	c := logsCmd()
	c.SetOut(&buf)
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No run logs yet.") {
		t.Fatalf("empty list: %q", buf.String())
	}
	buf.Reset()
	c = logsCmd()
	c.SetOut(&buf)
	c.SetErr(&buf)
	c.SetArgs([]string{"view", "1"})
	if err := c.Execute(); err == nil {
		t.Fatal("view of a missing log succeeded")
	}
}
