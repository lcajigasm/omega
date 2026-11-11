package gitops

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lcajigasm/omega/internal/runner"
)

func TestDefaultRoots(t *testing.T) {
	roots := DefaultRoots()
	t.Logf("Found %d default dev roots", len(roots))
}

func TestCheckRepo(t *testing.T) {
	tmp := t.TempDir()
	// Init test git repo
	res := runner.Run(context.Background(), "git -C "+tmp+" init", 0, nil)
	if res.ExitCode != 0 {
		t.Skip("git not available in test environment")
	}

	testFile := filepath.Join(tmp, "test.txt")
	_ = os.WriteFile(testFile, []byte("hello world"), 0644)

	st, err := CheckRepo(context.Background(), tmp)
	if err != nil {
		t.Fatalf("CheckRepo failed: %v", err)
	}
	if !st.HasUncommitted {
		t.Errorf("expected repo to have uncommitted changes")
	}
}
