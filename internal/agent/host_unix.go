//go:build !windows

package agent

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// dataVolume is where the user's files live. On macOS "/" is the sealed
// system volume: its "used" figure is the system alone, and the free space
// the user cares about is that of the Data volume (the APFS container
// shares it).
func dataVolume() string {
	if goos == "darwin" {
		if _, err := os.Stat("/System/Volumes/Data"); err == nil {
			return "/System/Volumes/Data"
		}
	}
	return "/"
}

// diskSpace is the size of a volume and what is free for the user on it.
func diskSpace(path string) (total, free uint64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, false
	}
	return uint64(st.Blocks) * uint64(st.Bsize), uint64(st.Bavail) * uint64(st.Bsize), true
}

// hardwareModel is the model identifier (Mac16,10) on macOS.
func hardwareModel() string {
	if goos != "darwin" {
		return ""
	}
	out, err := exec.Command("sysctl", "-n", "hw.model").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
