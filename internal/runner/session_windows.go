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
		// CREATE_NEW_PROCESS_GROUP, and CREATE_NO_WINDOW: the desktop app
		// has no console, so without it every command opened one of its
		// own, and closing that window killed the command.
		CreationFlags: 0x00000200 | 0x08000000,
	}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Kill()
	}
}
