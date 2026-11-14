//go:build unix

package customsteps

import (
	"os"
	"syscall"
)

// owner is the user a file belongs to.
func owner(info os.FileInfo) (uint32, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return st.Uid, true
}
