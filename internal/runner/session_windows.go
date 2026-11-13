//go:build windows

package runner

import (
	"os/exec"
	"strconv"
	"syscall"
)

// detach starts the command in its own process group. Windows has no
// session id to kill, and ending PowerShell leaves what it started running
// (winget, an installer, a build). taskkill /T ends the whole tree.
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
		kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		if err := kill.Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
