//go:build !windows

package agent

import (
	"fmt"
	"os"
	"syscall"
)

// fileID names a folder by device and inode, so two spellings of one folder
// are one.
func fileID(fi os.FileInfo, real string) string {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprintf("%d:%d", st.Dev, st.Ino)
	}
	return real
}
