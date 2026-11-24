//go:build unix

package runner

import (
	"os/exec"
	"syscall"
	"time"
)

// killGrace is how long a cancelled or timed-out command is given to stop
// after SIGTERM before its group gets SIGKILL. apt, dpkg, brew and
// softwareupdate release their locks on SIGTERM; killed outright they leave
// them behind, half done.
var killGrace = 5 * time.Second

// detach puts the command in its own session so it has no controlling
// terminal, and so a timeout or a cancel stops the whole process group:
// SIGTERM first, SIGKILL killGrace later for what is still there.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		pgid := cmd.Process.Pid
		err := syscall.Kill(-pgid, syscall.SIGTERM)
		if err != nil {
			return syscall.Kill(-pgid, syscall.SIGKILL)
		}
		time.AfterFunc(killGrace, func() {
			// ESRCH when the group is gone already: nothing to do.
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		})
		return nil
	}
}
