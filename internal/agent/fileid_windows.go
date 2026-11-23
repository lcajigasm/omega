//go:build windows

package agent

import (
	"os"
	"strings"
)

func fileID(_ os.FileInfo, real string) string { return strings.ToLower(real) }
