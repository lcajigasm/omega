package secscan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAuditSSH(t *testing.T) {
	// Smoke test
	findings, err := AuditSSH()
	if err != nil {
		t.Fatalf("AuditSSH returned error: %v", err)
	}
	t.Logf("Found %d SSH findings", len(findings))
}

func TestMaskSnippet(t *testing.T) {
	raw := "export OPENAI_API_KEY=sk-1234567890abcdef1234567890abcdef"
	masked := maskSnippet(raw)
	if masked == raw {
		t.Errorf("expected secret to be masked, got: %s", masked)
	}
}

func TestAuditPrivacy(t *testing.T) {
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
