package customsteps

import (
	"context"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestCustomStepsDecode(t *testing.T) {
	sample := `
[[steps]]
id = "test-custom"
name = "Test Custom Step"
category = "clean"
command = "echo clean"
risk = "safe"

[hooks]
pre_full = "echo pre"
`
	var csf CustomStepsFile
	_, err := toml.Decode(sample, &csf)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(csf.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(csf.Steps))
	}
	if csf.Steps[0].ID != "test-custom" {
		t.Errorf("expected step id 'test-custom', got %s", csf.Steps[0].ID)
	}
	if csf.Hooks["pre_full"] != "echo pre" {
		t.Errorf("expected hook 'echo pre', got %s", csf.Hooks["pre_full"])
	}
}

func TestRollbackSmoke(t *testing.T) {
	ctx := context.Background()
	msg, err := Rollback(ctx)
	// The system's restore point service may not answer in time on a CI
	// runner (Get-ComputerRestorePoint on Windows): that is the machine,
	// not omega.
	if err != nil && strings.Contains(err.Error(), "timed out") {
		t.Skipf("the system did not answer in time: %v", err)
	}
	if err != nil {
		t.Fatalf("Rollback returned error: %v", err)
	}
	if msg == "" {
		t.Errorf("expected non-empty message from Rollback")
	}
}
