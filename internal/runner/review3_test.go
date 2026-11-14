package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Credentials as configuration files and tools write them, not only as
// NAME=value.
func TestScrubMasksFileFormatsAndTokenShapes(t *testing.T) {
	for _, line := range []string{
		`machine github.com login x password ghp_abcdefghijklmnopqrstuvwxyz0123456789`,
		`    oauth_token: gho_abcdefghijklmnopqrstuvwxyz0123456789`,
		`"GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_abcdefghijklmnopqrstuvwxyz0123456789"`,
		`aws_secret_access_key = wJalrXUtnFEMIK7MDENGbPxRfiCYEXAMPLEKEY`,
		`GITHUB_PAT=github_pat_11ABCDEFG0abcdefghijklmnop`,
		`ANTHROPIC_API_KEY : sk-ant-api03-abcdefghijklmnopqrstuvwxyz`,
		`password = "hunter2hunter2"`,
		`{"access_token": "abcdef0123456789abcdef"}`,
		`Authorization: Bearer abcdef0123456789abcdef`,
		// Written in two halves: a whole token in the source stops the push.
		"export SLACK=xox" + "b-123456789012-1234567890123-AbCdEfGhIjKlMnOpQrStUvWx",
		`key AKIAIOSFODNN7EXAMPLE in use`,
	} {
		got := Scrub(line)
		for _, secret := range []string{"ghp_abc", "gho_abc", "wJalrXUtn", "github_pat_11", "sk-ant-api03", "hunter2", "abcdef0123456789", "1234567890123-AbCd", "AKIAIOSFODNN7EXAMPLE"} {
			if strings.Contains(got, secret) {
				t.Errorf("%q still shows %q: %q", line, secret, got)
			}
		}
	}
	key := "before\n-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\nAAAAB3NzaC1yc2EAAAADAQAB\n-----END OPENSSH PRIVATE KEY-----\nafter"
	got := Scrub(key)
	if strings.Contains(got, "b3Blbn") || strings.Contains(got, "AAAAB3") || !strings.HasPrefix(got, "before\n") || !strings.HasSuffix(got, "\nafter") {
		t.Errorf("a private key block: %q", got)
	}
	// What is not a secret stays readable.
	for _, line := range []string{
		`approval = "dangerous"`,
		`password_required: true`,
		`token_endpoint = /oauth/token`,
		`max_tokens: 4096`,
		`api_key_env = "$OPENAI_API_KEY"`,
		`skip_steps = ["trash"]`,
		`brew 4.4.1 ok`,
		`PATH=/usr/bin:/bin`,
	} {
		if got := Scrub(line); got != line {
			t.Errorf("%q became %q", line, got)
		}
	}
}

// A title, a clipboard write or a hyperlink in a command's output is not
// passed on to the terminal.
func TestStripANSIDropsOperatingSystemCommands(t *testing.T) {
	in := "a\x1b]0;title\x07b\x1b]52;c;ZXZpbA==\x1b\\c\x1b]8;;https://evil.example\x07link\x1b]8;;\x07 \x1b[31mred\x1b[0m"
	if got := stripANSI(in); got != "abclink red" {
		t.Errorf("stripANSI: %q", got)
	}
}

// A folder that brings its own .git/config does not get a program run by a
// command that only looks.
func TestGitDoesNotRunARepositorysMonitor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	repo := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	for _, args := range [][]string{{"init", "-q"}, {"config", "core.fsmonitor", "touch " + marker + "; false"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	os.WriteFile(filepath.Join(repo, "f"), []byte("x"), 0o644)

	// Without the setting git does run it: the test tests something.
	plain := exec.Command("git", "status", "--porcelain")
	plain.Dir = repo
	plain.Run()
	if _, err := os.Stat(marker); err != nil {
		t.Skip("this git does not run core.fsmonitor on status")
	}
	os.Remove(marker)

	res := Run(context.Background(), "cd "+repo+" && git status --porcelain", 30*time.Second, nil)
	if _, err := os.Stat(marker); err == nil {
		t.Errorf("git status ran the repository's fsmonitor:\n%s", res.Output)
	}
	if !strings.Contains(res.Output, "f") {
		t.Errorf("git status no longer works: %q", res.Output)
	}
}

// A repository that names programs in its own configuration is recognised,
// from anywhere inside it; an ordinary one is not.
func TestGitConfigRuns(t *testing.T) {
	plain := t.TempDir()
	os.MkdirAll(filepath.Join(plain, ".git"), 0o755)
	os.WriteFile(filepath.Join(plain, ".git", "config"), []byte("[core]\n\trepositoryformatversion = 0\n\tbare = false\n"+
		"[remote \"origin\"]\n\turl = https://github.com/a/b.git\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n"+
		"[branch \"main\"]\n\tremote = origin\n\tmerge = refs/heads/main\n[user]\n\tname = Ana\n"), 0o644)
	os.MkdirAll(filepath.Join(plain, "src", "deep"), 0o755)
	if GitConfigRuns(filepath.Join(plain, "src", "deep")) {
		t.Error("an ordinary repository was taken for one that runs programs")
	}
	if GitConfigRuns(t.TempDir()) {
		t.Error("a folder that is no repository")
	}
	for _, extra := range []string{
		"[filter \"x\"]\n\tclean = touch /tmp/pwned; cat\n",
		"[diff \"y\"]\n\ttextconv = sh -c x\n",
		"[core]\n\tfsmonitor = touch x\n",
		"[core]\n\tpager = sh -c x\n",
		"[alias]\n\tstatus = !sh -c x\n",
		"[include]\n\tpath = /tmp/evil.config\n",
		"[core]\n\thooksPath = /tmp/hooks\n",
	} {
		repo := t.TempDir()
		os.MkdirAll(filepath.Join(repo, ".git"), 0o755)
		os.MkdirAll(filepath.Join(repo, "a"), 0o755)
		os.WriteFile(filepath.Join(repo, ".git", "config"), []byte("[core]\n\tbare = false\n"+extra), 0o644)
		if !GitConfigRuns(filepath.Join(repo, "a")) {
			t.Errorf("not recognised: %q", extra)
		}
	}
	// A work tree: .git is a file that points at the real folder.
	real := t.TempDir()
	os.WriteFile(filepath.Join(real, "config"), []byte("[filter \"x\"]\n\tclean = cat\n"), 0o644)
	tree := t.TempDir()
	os.WriteFile(filepath.Join(tree, ".git"), []byte("gitdir: "+real+"\n"), 0o644)
	if !GitConfigRuns(tree) {
		t.Error("a work tree whose repository runs programs was not recognised")
	}
}
