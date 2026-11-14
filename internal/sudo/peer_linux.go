package sudo

import (
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// procRoot is where the kernel describes the processes. Tests replace it.
var procRoot = "/proc"

// peerPID is the process that connected.
func peerPID(conn *net.UnixConn) (int, bool) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, false
	}
	pid, ok := 0, false
	raw.Control(func(fd uintptr) {
		if c, err := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED); err == nil && c.Pid > 0 {
			pid, ok = int(c.Pid), true
		}
	})
	return pid, ok
}

// startedBySudo reports whether the parent of a process is sudo itself: a
// process called sudo whose effective user is root. Only the real, setuid
// sudo can be that for a user who is not root.
func startedBySudo(pid int) bool {
	stat, err := os.ReadFile(procRoot + "/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	// "pid (name) state ppid …": the name may hold spaces and brackets.
	i := strings.LastIndexByte(string(stat), ')')
	if i < 0 {
		return false
	}
	f := strings.Fields(string(stat)[i+1:])
	if len(f) < 2 {
		return false
	}
	ppid, err := strconv.Atoi(f[1])
	if err != nil || ppid <= 1 {
		return false
	}
	status, err := os.ReadFile(procRoot + "/" + strconv.Itoa(ppid) + "/status")
	if err != nil {
		return false
	}
	name, root := "", false
	for _, line := range strings.Split(string(status), "\n") {
		key, val, _ := strings.Cut(line, ":")
		switch key {
		case "Name":
			name = strings.TrimSpace(val)
		case "Uid":
			// real, effective, saved, filesystem
			ids := strings.Fields(val)
			root = len(ids) > 1 && ids[1] == "0"
		}
	}
	return root && name == "sudo"
}
