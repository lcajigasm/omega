//go:build !windows

package cli

import (
	"os"
	"syscall"
)

// systemFree is the free space of the disk that holds the system's data, in
// bytes; ok is false when it cannot be read.
func systemFree() (free uint64, ok bool) {
	path := "/"
	if _, err := os.Stat("/System/Volumes/Data"); err == nil {
		path = "/System/Volumes/Data"
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, false
	}
	return uint64(st.Bavail) * uint64(st.Bsize), true
}
