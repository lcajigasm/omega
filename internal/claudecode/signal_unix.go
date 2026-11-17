//go:build !windows

package claudecode

import (
	"errors"
	"syscall"
)

func terminate(pid int) { syscall.Kill(pid, syscall.SIGTERM) }

func forceKill(pid int) { syscall.Kill(pid, syscall.SIGKILL) }

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
