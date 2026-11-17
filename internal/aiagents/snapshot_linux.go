package aiagents

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// On Linux /proc has everything: the environment of the user's own
// processes is readable in /proc/<pid>/environ.

const proc = "/proc"

func snapshot() (snap, error) {
	s := snap{procs: map[int]*raw{}}
	entries, err := os.ReadDir(proc)
	if err != nil {
		return s, err
	}
	boot := bootTime()
	uptime := uptimeSeconds()
	uid := uint32(os.Getuid())
	for _, e := range entries {
		pid := atoi(e.Name())
		if pid == 0 {
			continue
		}
		dir := filepath.Join(proc, e.Name())
		stat, err := os.ReadFile(filepath.Join(dir, "stat"))
		if err != nil {
			continue
		}
		p := parseStat(pid, string(stat))
		if p == nil {
			continue
		}
		if p.startTicks > 0 && uptime > 0 {
			started := float64(p.startTicks) / clockTicks
			p.age = max(0, int(uptime-started))
			if !boot.IsZero() {
				p.start = boot.Add(time.Duration(started * float64(time.Second)))
			}
		}
		if info, err := os.Stat(dir); err == nil {
			if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Uid == uid {
				p.mine = true
			}
		}
		if exe, err := os.Readlink(filepath.Join(dir, "exe")); err == nil {
			p.exe = strings.TrimSuffix(exe, " (deleted)")
		} else {
			p.exe = p.comm
		}
		// A program can rename itself (node does, to "claude"); the name in
		// stat is what it calls itself.
		if identify(p.exe, "") == nil && identify(p.comm, "") != nil {
			p.exe = p.comm
		}
		if cmd, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
			p.command = strings.TrimSpace(string(bytes.ReplaceAll(cmd, []byte{0}, []byte{' '})))
		}
		if p.command == "" {
			p.command = "[" + p.comm + "]"
		}
		if p.mine {
			if env, err := os.ReadFile(filepath.Join(dir, "environ")); err == nil {
				p.env = markers(env)
			}
		}
		p.rss = rssOf(dir)
		s.procs[pid] = &p.raw
	}
	return s, nil
}

type linuxProc struct {
	raw
	comm       string
	startTicks uint64
}

// clockTicks is USER_HZ, 100 on every Linux omega runs on.
const clockTicks = 100

// parseStat reads /proc/<pid>/stat. The name is in parentheses and may
// itself hold spaces and parentheses, so the fields are counted from the
// last ")".
func parseStat(pid int, stat string) *linuxProc {
	open, end := strings.IndexByte(stat, '('), strings.LastIndexByte(stat, ')')
	if open < 0 || end < open {
		return nil
	}
	f := strings.Fields(stat[end+1:])
	// state ppid ... starttime is the 22nd field of the line, the 20th here.
	if len(f) < 20 {
		return nil
	}
	start, _ := strconv.ParseUint(f[19], 10, 64)
	return &linuxProc{raw: raw{pid: pid, ppid: atoi(f[1])}, comm: stat[open+1 : end], startTicks: start}
}

// markers reads the three marker variables from a NUL-separated environment.
func markers(env []byte) map[string]string {
	keys := envKeys()
	var out map[string]string
	for _, kv := range bytes.Split(env, []byte{0}) {
		k, v, ok := strings.Cut(string(kv), "=")
		if !ok || !keys[k] {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[k] = v
	}
	return out
}

func rssOf(dir string) uint64 {
	data, err := os.ReadFile(filepath.Join(dir, "status"))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(line, "VmRSS:"); ok {
			f := strings.Fields(rest)
			if len(f) > 0 {
				return uint64(atoi(f[0])) * 1024
			}
		}
	}
	return 0
}

func uptimeSeconds() float64 {
	data, err := os.ReadFile(filepath.Join(proc, "uptime"))
	if err != nil {
		return 0
	}
	f := strings.Fields(string(data))
	if len(f) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(f[0], 64)
	return v
}

func bootTime() time.Time {
	data, err := os.ReadFile(filepath.Join(proc, "stat"))
	if err != nil {
		return time.Time{}
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(line, "btime "); ok {
			if n, err := strconv.ParseInt(strings.TrimSpace(rest), 10, 64); err == nil {
				return time.Unix(n, 0)
			}
		}
	}
	return time.Time{}
}

func cwdOf(pid int) string {
	cwd, _ := os.Readlink(filepath.Join(proc, strconv.Itoa(pid), "cwd"))
	return cwd
}

func commandOf(pid int) string {
	cmd, err := os.ReadFile(filepath.Join(proc, strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return ""
	}
	if s := strings.TrimSpace(string(bytes.ReplaceAll(cmd, []byte{0}, []byte{' '}))); s != "" {
		return s
	}
	stat, err := os.ReadFile(filepath.Join(proc, strconv.Itoa(pid), "stat"))
	if p := parseStat(pid, string(stat)); err == nil && p != nil {
		return "[" + p.comm + "]"
	}
	return ""
}
