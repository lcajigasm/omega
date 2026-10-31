//go:build windows

package sudo

import "os/exec"

func setSession(cmd *exec.Cmd) {}
