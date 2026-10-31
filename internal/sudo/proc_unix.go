//go:build unix

package sudo

import (
	"os/exec"
	"syscall"
)

func setSession(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
