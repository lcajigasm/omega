//go:build !darwin && !linux

package sudo

import "net"

// On the systems omega is not released for there is no check of who asks.
func peerPID(*net.UnixConn) (int, bool) { return 0, false }

func startedBySudo(int) bool { return true }
