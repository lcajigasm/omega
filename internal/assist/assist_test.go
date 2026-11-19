package assist

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeHome points omega's state, its configuration and the definitions at a
// temporary home with nothing installed but what a test makes.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".state"))
	old := newEnv
	t.Cleanup(func() { newEnv = old })
	newEnv = func() *env {
		return &env{
			home: home, goos: "darwin", apps: filepath.Join(home, "Apps"),
			look:    func(string) (string, error) { return "", exec.ErrNotFound },
			running: func(string) bool { return false },
			omega:   "/usr/local/bin/omega",
			budgets: loadBudgets(),
		}
	}
	return home
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestNothingInstalledHasNoChecks(t *testing.T) {
	fakeHome(t)
	r := Scan()
	if len(r.Tools) != len(IDs()) {
		t.Fatalf("%d tools, want %d", len(r.Tools), len(IDs()))
	}
	for _, tool := range r.Tools {
		if tool.Installed || len(tool.Checks) > 0 {
			t.Errorf("%s: installed=%v with %d checks on an empty home", tool.ID, tool.Installed, len(tool.Checks))
		}
	}
}

func TestClaudeCodeSettingsKeepTheRest(t *testing.T) {
	home := fakeHome(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	write(t, settings, "{\n  \"theme\": \"dark\",\n  \"permissions\": {\"deny\": [\"Read(./.env)\"], \"allow\": [\"Bash(ls)\"]}\n}\n")

	r := Scan()
	c, ok := r.Check("claude-code.transcripts")
	if !ok || c.OK || c.Current != "30 days" {
		t.Fatalf("transcripts check: %+v", c)
	}
	if _, err := Apply("claude-code.transcripts", "0"); err == nil {
		t.Error("0 days was accepted: Claude Code would delete every conversation")
	}
	if _, err := Apply("claude-code.transcripts", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply("claude-code.secrets", ""); err != nil {
		t.Fatal(err)
	}
	// Twice adds nothing.
	if _, err := Apply("claude-code.secrets", ""); err != nil {
		t.Fatal(err)
	}
	got := read(t, settings)
	for _, want := range []string{`"theme": "dark"`, `"cleanupPeriodDays": 14`, `"Bash(ls)"`, `"Read(~/.ssh/**)"`} {
		if !strings.Contains(got, want) {
			t.Errorf("settings.json lacks %s:\n%s", want, got)
		}
	}
	if strings.Count(got, `"Read(./.env)"`) != 1 {
		t.Errorf("a deny rule was added twice:\n%s", got)
	}
	baks, _ := filepath.Glob(filepath.Join(BackupDir(), "claude-code", "settings.json.*"))
	if len(baks) == 0 {
		t.Error("no copy of settings.json was kept")
	}
	r = Scan()
	if tool, _ := r.Find("claude"); tool.Pending != 1 { // only the MCP server is left
		t.Errorf("pending after applying: %d", tool.Pending)
	}
}

func TestJSONWithCommentsIsLeftAlone(t *testing.T) {
	home := fakeHome(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	text := "{\n  // mine\n  \"theme\": \"dark\"\n}\n"
	write(t, settings, text)
	if _, err := Apply("claude-code.transcripts", "7"); err == nil {
		t.Error("a JSON file with comments was rewritten")
	}
	if read(t, settings) != text {
		t.Error("the file changed")
	}
}

func TestCodexTOML(t *testing.T) {
	home := fakeHome(t)
	cfg := filepath.Join(home, ".codex", "config.toml")
	write(t, cfg, "# my settings\nmodel = \"gpt\"\n\n[history]\npersistence = \"save-all\"\n\n[features]\nmemories = true\n")
	if _, err := Apply("codex.history", "20MB"); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply("codex.mcp", ""); err != nil {
		t.Fatal(err)
	}
	got := read(t, cfg)
	for _, want := range []string{"# my settings", "persistence = \"save-all\"\nmax_bytes = 20971520\n", "[features]\nmemories = true", "[mcp_servers.omega]\ncommand = \"/usr/local/bin/omega\"\nargs = [\"mcp\"]"} {
		if !strings.Contains(got, want) {
			t.Errorf("config.toml lacks %q:\n%s", want, got)
		}
	}
	// Setting it again replaces the line.
	if _, err := Apply("codex.history", "10MB"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, cfg); strings.Count(got, "max_bytes") != 1 || !strings.Contains(got, "max_bytes = 10485760") {
		t.Errorf("max_bytes not replaced:\n%s", got)
	}
}

func TestBrokenTOMLIsLeftAlone(t *testing.T) {
	home := fakeHome(t)
	cfg := filepath.Join(home, ".codex", "config.toml")
	write(t, cfg, "model = \n")
	if _, err := Apply("codex.history", "20MB"); err == nil {
		t.Error("a TOML file that does not parse was rewritten")
	}
}

func TestOldVersionsKeepTheLinkedOne(t *testing.T) {
	home := fakeHome(t)
	versions := filepath.Join(home, ".local", "share", "cursor-agent", "versions")
	for i, v := range []string{"2026.01.01", "2026.02.01", "2026.03.01"} {
		write(t, filepath.Join(versions, v, "cursor-agent"), strings.Repeat("x", 2<<20))
		ts := time.Now().Add(time.Duration(i-5) * time.Hour)
		os.Chtimes(filepath.Join(versions, v), ts, ts)
	}
	bin := filepath.Join(home, ".local", "bin")
	os.MkdirAll(bin, 0o755)
	// The link points at the middle one, not the newest.
	if err := os.Symlink(filepath.Join(versions, "2026.02.01", "cursor-agent"), filepath.Join(bin, "cursor-agent")); err != nil {
		t.Skip("no symbolic links here:", err) // Windows without developer mode
	}
	r := Scan()
	c, ok := r.Check("cursor.cli-versions")
	if !ok || c.OK || c.Free == 0 {
		t.Fatalf("cli versions: %+v", c)
	}
	if _, err := Apply("cursor.cli-versions", ""); err != nil {
		t.Fatal(err)
	}
	for v, want := range map[string]bool{"2026.01.01": false, "2026.02.01": true, "2026.03.01": false} {
		if isDir(filepath.Join(versions, v)) != want {
			t.Errorf("%s kept=%v, want %v", v, !want, want)
		}
	}
}

func TestBudgetIsTheUsers(t *testing.T) {
	home := fakeHome(t)
	sessions := filepath.Join(home, ".codex", "sessions", "2026", "01")
	for i := 0; i < 4; i++ {
		p := filepath.Join(sessions, "s"+string(rune('a'+i))+".jsonl")
		write(t, p, strings.Repeat("x", 1<<20))
		ts := time.Now().Add(time.Duration(i-10) * time.Hour)
		os.Chtimes(p, ts, ts)
	}
	r := Scan()
	c, _ := r.Check("codex.sessions")
	if !c.OK || c.Free != 0 {
		t.Fatalf("a folder without a budget is due: %+v", c)
	}
	if out, _ := Enforce(true); len(out) != 0 {
		t.Errorf("enforce without budgets plans %v", out)
	}
	if _, err := Apply("codex.sessions", "2.5MB"); err != nil {
		t.Fatal(err)
	}
	if _, err := Enforce(false); err != nil {
		t.Fatal(err)
	}
	left, _ := filepath.Glob(filepath.Join(sessions, "*.jsonl"))
	if len(left) != 2 {
		t.Fatalf("%d sessions left, want the 2 newest", len(left))
	}
	for _, p := range left {
		if b := filepath.Base(p); b == "sa.jsonl" || b == "sb.jsonl" {
			t.Errorf("the oldest session %s was kept", b)
		}
	}
	if _, err := Apply("codex.sessions", "off"); err != nil {
		t.Fatal(err)
	}
	if c, _ := Scan().Check("codex.sessions"); c.Current != "no budget" {
		t.Errorf("budget not removed: %s", c.Current)
	}
}

func TestOpenAppIsNotEmptied(t *testing.T) {
	home := fakeHome(t)
	os.MkdirAll(filepath.Join(home, "Apps", "Claude.app", "Contents"), 0o755)
	vm := filepath.Join(home, "Library", "Application Support", "Claude", "vm_bundles")
	write(t, filepath.Join(vm, "claudevm.bundle", "rootfs.img"), strings.Repeat("x", 3<<20))
	old := newEnv
	newEnv = func() *env { e := old(); e.running = func(string) bool { return true }; return e }
	if _, err := Apply("claude-desktop.vm", "1MB"); !errors.Is(err, ErrBlocked) {
		t.Fatalf("setting a budget on an open app: %v", err)
	}
	newEnv = old
	if _, err := Apply("claude-desktop.vm", "1MB"); err != nil {
		t.Fatal(err)
	}
	newEnv = func() *env { e := old(); e.running = func(string) bool { return true }; return e }
	out, err := Enforce(false)
	if err != nil || len(out) != 1 || !strings.HasPrefix(out[0], "skip claude-desktop.vm") {
		t.Fatalf("enforce with Claude open: %v %v", out, err)
	}
	if !exists(filepath.Join(vm, "claudevm.bundle")) {
		t.Fatal("the VM of an open Claude was removed")
	}
	newEnv = old
	if _, err := Enforce(false); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(vm, "claudevm.bundle")) || !isDir(vm) {
		t.Error("the VM folder was not emptied, or the folder itself went")
	}
}

func TestDockerDiskNeverBelowWhatItHolds(t *testing.T) {
	home := fakeHome(t)
	os.MkdirAll(filepath.Join(home, "Apps", "Docker.app", "Contents"), 0o755)
	write(t, filepath.Join(home, "Library", "Containers", "com.docker.docker", "Data", "vms", "0", "data", "Docker.raw"), strings.Repeat("x", 1<<20))
	if _, err := Apply("docker.disk", "1GB"); err == nil {
		t.Error("a disk limit below what Docker holds plus a margin was accepted")
	}
	if _, err := Apply("docker.disk", "40GB"); err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(home, "Library", "Group Containers", "group.com.docker", "settings-store.json"))
	if !strings.Contains(got, `"DiskSizeMiB": 40960`) {
		t.Errorf("settings-store.json: %s", got)
	}
}

func TestWithinRefusesTheRoot(t *testing.T) {
	for _, c := range []struct {
		p, root string
		want    bool
	}{
		{"/a/b/c", "/a/b", true},
		{"/a/b", "/a/b", false},
		{"/a", "/a/b", false},
		{"/a/bc", "/a/b", false},
		{"/a/b/../x", "/a/b", false},
	} {
		if got := within(filepath.FromSlash(c.p), filepath.FromSlash(c.root)); got != c.want {
			t.Errorf("within(%s, %s) = %v", c.p, c.root, got)
		}
	}
}

func TestParseSize(t *testing.T) {
	for in, want := range map[string]int64{"8GB": 8 << 30, "8": 8 << 30, "500MB": 500 << 20, "1.5 gb": 3 << 29, "20m": 20 << 20, "1,5G": 3 << 29} {
		got, err := ParseSize(in)
		if err != nil || got != want {
			t.Errorf("ParseSize(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "-1GB", "0", "lots", "8 PB"} {
		if _, err := ParseSize(in); err == nil {
			t.Errorf("ParseSize(%q) accepted", in)
		}
	}
}

func TestFindByAlias(t *testing.T) {
	fakeHome(t)
	r := Scan()
	for in, want := range map[string]string{"chatgpt": "codex", "Claude Code": "claude-code", "grok bot": "grok", "brew": "homebrew", "Docker Desktop": "docker"} {
		if tool, ok := r.Find(in); !ok || tool.ID != want {
			t.Errorf("Find(%q) = %v", in, tool)
		}
	}
}
