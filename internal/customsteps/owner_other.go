//go:build !unix

package customsteps

import "os"

func owner(os.FileInfo) (uint32, bool) { return 0, false }
