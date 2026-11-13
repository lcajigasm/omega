package secscan

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// home gives the test an empty home directory.
func home(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir) // the home directory on Windows
	return dir
}

func TestAuditSSH(t *testing.T) {
	dir := home(t)
	if findings, err := AuditSSH(); err != nil || len(findings) != 0 {
		t.Fatalf("no ~/.ssh: %+v, %v", findings, err)
	}
	ssh := filepath.Join(dir, ".ssh")
	os.MkdirAll(ssh, 0o700)
	os.WriteFile(filepath.Join(ssh, "id_open"), []byte("key"), 0o644)
	os.WriteFile(filepath.Join(ssh, "id_closed"), []byte("key"), 0o600)
	// A key kept elsewhere and linked: ssh reads the file, not the link.
	os.MkdirAll(filepath.Join(dir, "dotfiles"), 0o700)
	os.WriteFile(filepath.Join(dir, "dotfiles", "id_linked"), []byte("key"), 0o600)
	linked := os.Symlink(filepath.Join(dir, "dotfiles", "id_linked"), filepath.Join(ssh, "id_linked")) == nil
	config := "Host a\n  IdentityFile \"~/.ssh/id_closed\"\nHost b\n  IdentityFile=~/.ssh/id_closed\n" +
		"Host c\n  IdentityFile ~/.ssh/%h_key\nHost d\n  IdentityFile none\nHost e\n  IdentityFile \"~/.ssh/gone key\"\n"
	os.WriteFile(filepath.Join(ssh, "config"), []byte(config), 0o600)

	findings, err := AuditSSH()
	if err != nil {
		t.Fatal(err)
	}
	var open, missing int
	for _, f := range findings {
		switch {
		case strings.Contains(f.Issue, "insecure permissions"):
			open++
			if filepath.Base(f.Path) != "id_open" {
				t.Errorf("a key with safe permissions was reported: %+v (linked: %v)", f, linked)
			}
		case strings.Contains(f.Issue, "non-existent"):
			missing++
			if !strings.Contains(f.Issue, "gone key") {
				t.Errorf("an IdentityFile that exists or cannot be known was reported: %+v", f)
			}
		}
	}
	wantOpen := 1
	if runtime.GOOS == "windows" {
		wantOpen = 0 // permissions are not looked at there
	}
	if open != wantOpen || missing != 1 {
		t.Errorf("%d open keys and %d missing files reported, want %d and 1: %+v", open, missing, wantOpen, findings)
	}
}

func TestScanSecrets(t *testing.T) {
	dir := home(t)
	projKey := "sk-proj-" + strings.Repeat("aB3_-", 12) + "Zz9"
	svcKey := "sk-svcacct-" + strings.Repeat("Qw8", 20)
	history := "ls\n" +
		"cat <<EOF " + strings.Repeat("x", 70*1024) + "\n" + // longer than a Scanner takes
		"export OPENAI_API_KEY=" + projKey + "\n" +
		"curl -H 'Authorization: Bearer " + svcKey + "' https://example.com\n" +
		// The key starts 60 characters in: cutting the line at 80 first
		// used to leave part of it unmatched and on show.
		strings.Repeat("a", 59) + " sk-" + strings.Repeat("k", 48) + " tail"
	if err := os.WriteFile(filepath.Join(dir, ".zsh_history"), []byte(history), 0o600); err != nil {
		t.Fatal(err)
	}
	findings, err := ScanSecrets()
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 3 || findings[0].LineNum != 3 || findings[1].LineNum != 4 || findings[2].LineNum != 5 {
		t.Fatalf("findings: %+v", findings)
	}
	for _, f := range findings {
		if f.Type != "OpenAI API Key" {
			t.Errorf("type: %+v", f)
		}
		for _, secret := range []string{"aB3_-", "Qw8Qw8", "kkkk"} {
			if strings.Contains(f.Snippet, secret) {
				t.Errorf("part of a key is shown: %q", f.Snippet)
			}
		}
		if len([]rune(f.Snippet)) > 80 {
			t.Errorf("snippet of %d characters", len([]rune(f.Snippet)))
		}
	}
}

// Nothing is said about telemetry that was not read.
func TestTelemetryReadsTheSetting(t *testing.T) {
	oldOS, oldOut, oldPop := goos, output, popconFile
	defer func() { goos, output, popconFile = oldOS, oldOut, oldPop }()

	goos = "darwin"
	for answer, want := range map[string]string{"1\n": "is on", "0\n": "is off", "": "could not be read"} {
		output = func(string, ...string) ([]byte, error) {
			if answer == "" {
				return nil, errors.New("does not exist")
			}
			return []byte(answer), nil
		}
		if got := telemetry(); len(got) != 1 || !strings.Contains(got[0], want) {
			t.Errorf("AutoSubmit %q: %q, want %q", answer, got, want)
		}
	}
	goos = "windows"
	if got := telemetry(); len(got) != 1 || !strings.Contains(got[0], "not checked") || strings.Contains(got[0], "active") {
		t.Errorf("windows: %q", got)
	}
	goos = "linux"
	popconFile = filepath.Join(t.TempDir(), "popularity-contest.conf")
	if got := telemetry(); len(got) != 0 {
		t.Errorf("no popularity-contest: %q", got)
	}
	os.WriteFile(popconFile, []byte("MY_HOSTID=\"x\"\nPARTICIPATE=\"no\"\n"), 0o644)
	if got := telemetry(); len(got) != 1 || !strings.Contains(got[0], "switched off") {
		t.Errorf("PARTICIPATE=no: %q", got)
	}
	os.WriteFile(popconFile, []byte("PARTICIPATE=\"yes\"\n"), 0o644)
	if got := telemetry(); len(got) != 1 || !strings.Contains(got[0], "is on") {
		t.Errorf("PARTICIPATE=yes: %q", got)
	}
}

func TestMaskSnippet(t *testing.T) {
	raw := "export OPENAI_API_KEY=sk-1234567890abcdef1234567890abcdef"
	masked := maskSnippet(raw)
	if masked == raw {
		t.Errorf("expected secret to be masked, got: %s", masked)
	}
}

func TestAuditPrivacy(t *testing.T) {
	home(t)
	old := output
	defer func() { output = old }()
	output = func(string, ...string) ([]byte, error) { return []byte("0\n"), nil }
	audit, err := AuditPrivacy()
	if err != nil {
		t.Fatalf("AuditPrivacy returned error: %v", err)
	}
	if audit == nil {
		t.Fatalf("expected non-nil PrivacyAudit")
	}
}

func TestAuditSSHConfigFile(t *testing.T) {
	tmp := t.TempDir()
	conf := filepath.Join(tmp, "config")
	err := os.WriteFile(conf, []byte("Host test\n  IdentityFile /does/not/exist/id_ed25519\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}
	var findings []SSHFinding
	auditSSHConfigFile(conf, &findings)
	if len(findings) != 1 {
		t.Errorf("expected 1 finding for missing IdentityFile, got %d", len(findings))
	}
}
