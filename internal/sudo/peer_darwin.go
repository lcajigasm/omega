package sudo

import (
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// solLocal and localPeerPID ask a Unix socket for the process at its other
// end (sys/un.h).
const (
	solLocal     = 0
	localPeerPID = 2
)

// peerPID is the process that connected.
func peerPID(conn *net.UnixConn) (int, bool) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, false
	}
	pid, ok := 0, false
	raw.Control(func(fd uintptr) {
		if v, err := syscall.GetsockoptInt(int(fd), solLocal, localPeerPID); err == nil && v > 0 {
			pid, ok = v, true
		}
	})
	return pid, ok
}

// startedBySudo reports whether the parent of a process is sudo itself: a
// process called sudo that runs as root. Only the real, setuid sudo can be
// that for a user who is not root.
func startedBySudo(pid int) bool {
	out, err := exec.Command("/bin/ps", "-o", "ppid=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return false
	}
	ppid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || ppid <= 1 {
		return false
	}
	out, err = exec.Command("/bin/ps", "-o", "uid=,comm=", "-p", strconv.Itoa(ppid)).Output()
	if err != nil {
		return false
	}
	uid, comm, found := strings.Cut(strings.TrimSpace(string(out)), " ")
	return found && uid == "0" && filepath.Base(strings.TrimSpace(comm)) == "sudo"
}
