//go:build windows

package runner

import (
	"os/exec"
	"syscall"
)

// detach starts the command in its own process group. Windows has no
// session id to kill; cancelling stops the process CommandContext started.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000200, // CREATE_NEW_PROCESS_GROUP
	}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Kill()
	}
}
