//go:build unix

package runner

import (
	"os/exec"
	"syscall"
)

// detach puts the command in its own session so it has no controlling
// terminal, and so a timeout kills the whole process group.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
