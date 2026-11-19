//go:build !windows

package assist

import (
	"io/fs"
	"syscall"
)

func onDisk(fi fs.FileInfo) int64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return int64(st.Blocks) * 512
	}
	return fi.Size()
}
