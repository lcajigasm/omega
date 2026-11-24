package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/runner"
)

// No snapshots is said, not an empty list; snapper is never asked in a way
// that waits for a password.
func TestRollbackSaysNone(t *testing.T) {
	oldRun, oldHas := rollbackRun, rollbackHas
	t.Cleanup(func() { rollbackRun, rollbackHas = oldRun, oldHas })
	var ran []string
	rollbackRun = func(_ context.Context, c string) runner.Result {
		ran = append(ran, c)
		return runner.Result{Output: "Snapshots for disk /:\n"}
	}
	l := listSnapshots(context.Background(), "darwin")
	var out bytes.Buffer
	writeSnapshots(&out, l)
	if len(l.Snapshots) != 0 || !strings.Contains(out.String(), "No snapshots") {
		t.Errorf("%+v\n%s", l, out.String())
	}
	rollbackHas = func(string) bool { return true }
	ran = nil
	listSnapshots(context.Background(), "linux")
	if len(ran) != 1 || !strings.Contains(ran[0], "sudo -n") {
		t.Errorf("snapper asked as %q", ran)
	}
}
