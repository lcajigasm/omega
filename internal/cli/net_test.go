package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/netdiag"
)

// fakePorts makes port 5000 held by ControlCenter (on IPv4 and IPv6) and
// records what was stopped.
func fakePorts(t *testing.T) *[]int {
	t.Helper()
	oldFind, oldStop := findPorts, stopProcess
	t.Cleanup(func() { findPorts, stopProcess = oldFind, oldStop })
	findPorts = func(_ context.Context, port int, _ string) ([]netdiag.PortInfo, error) {
		if port != 5000 {
			return nil, nil
		}
		return []netdiag.PortInfo{{Port: 5000, PID: 673, Process: "ControlCenter", Proto: "TCP"},
			{Port: 5000, PID: 673, Process: "ControlCenter", Proto: "TCP"}}, nil
	}
	var stopped []int
	stopProcess = func(_ context.Context, pid int, grace time.Duration) (bool, error) {
		stopped = append(stopped, pid)
		return false, nil
	}
	return &stopped
}

// net kill says what it ends, asks, and refuses without a terminal or --yes.
func TestNetKillAsks(t *testing.T) {
	stopped := fakePorts(t)
	var out bytes.Buffer
	err := killPort(context.Background(), &out, 5000, false, false, nil)
	if err == nil || !strings.Contains(err.Error(), "--yes") || len(*stopped) != 0 {
		t.Fatalf("without a terminal: %v, stopped %v", err, *stopped)
	}
	if !strings.Contains(out.String(), "PID 673 ControlCenter on :5000") {
		t.Errorf("what is ended is not said:\n%s", out.String())
	}

	asked := ""
	out.Reset()
	err = killPort(context.Background(), &out, 5000, false, true, func(q string) bool { asked = q; return false })
	if err != nil || asked == "" || len(*stopped) != 0 {
		t.Fatalf("a no: %v, asked %q, stopped %v", err, asked, *stopped)
	}

	err = killPort(context.Background(), &out, 5000, true, false, nil)
	if err != nil || len(*stopped) != 1 || (*stopped)[0] != 673 {
		t.Fatalf("--yes: %v, stopped %v (one process, once)", err, *stopped)
	}
	if err := killPort(context.Background(), &out, 6000, true, false, nil); err == nil {
		t.Error("a free port was not an error")
	}
}
