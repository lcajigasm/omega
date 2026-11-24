package netdiag

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// osSignal asks with taskkill, which sends the window a close request, and
// forces with taskkill /F.
func osSignal(ctx context.Context, pid int, force bool) error {
	cmd := fmt.Sprintf("taskkill /PID %d", pid)
	if force {
		cmd = fmt.Sprintf("taskkill /F /PID %d", pid)
	}
	if res := run(ctx, cmd, 5*time.Second); res.ExitCode != 0 {
		return fmt.Errorf("%s", strings.TrimSpace(res.Output))
	}
	return nil
}

func osAlive(pid int) bool {
	res := run(context.Background(), fmt.Sprintf(`tasklist /FI "PID eq %d" /NH`, pid), 5*time.Second)
	for _, f := range strings.Fields(res.Output) {
		if f == strconv.Itoa(pid) {
			return true
		}
	}
	return false
}
