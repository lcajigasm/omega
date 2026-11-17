package claudecode

import (
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// On macOS ps is the only way in: its "e" flag appends the environment to
// the command line, and the system shows it only for the user's own
// processes.

func ps(env []string, args ...string) string {
	cmd := exec.Command("/bin/ps", args...)
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	out, _ := cmd.Output()
	return string(out)
}

var markerRE = regexp.MustCompile(`(?:^|\s)(` + envMarker + `|` + envSession + `|` + envPID + `)=(\S+)`)

func snapshot() (snap, error) {
	uid := strconv.Itoa(os.Getuid())
	s := snap{procs: map[int]*raw{}}
	for _, line := range strings.Split(ps(nil, "-A", "-o", "pid=,ppid=,rss=,comm="), "\n") {
		f := fields(line, 4)
		if f == nil {
			continue
		}
		pid := atoi(f[0])
		if pid == 0 {
			continue
		}
		s.procs[pid] = &raw{pid: pid, ppid: atoi(f[1]), rss: uint64(atoi(f[2])) * 1024, exe: f[3]}
	}
	// Claude Code records its own start in UTC and in English, so ask ps
	// for the same.
	utc := []string{"TZ=UTC", "LC_ALL=C"}
	for _, line := range strings.Split(ps(utc, "ww", "-U", uid, "-o", "pid=,etime=,lstart=,command="), "\n") {
		// lstart is five words: "Tue Nov 17 16:25:01 2026".
		f := fields(line, 8)
		if f == nil {
			continue
		}
		p := s.procs[atoi(f[0])]
		if p == nil {
			continue
		}
		p.mine = true
		p.age = elapsed(f[1])
		p.lstart = strings.Join(f[2:7], " ")
		if t, err := time.Parse("Mon Jan 2 15:04:05 2006", p.lstart); err == nil {
			p.start = t
		}
		p.command = f[7]
	}
	for _, line := range strings.Split(ps(nil, "eww", "-U", uid, "-o", "pid=,command="), "\n") {
		if !strings.Contains(line, envMarker+"=") {
			continue
		}
		f := fields(line, 2)
		if f == nil {
			continue
		}
		p := s.procs[atoi(f[0])]
		if p == nil || !p.mine {
			continue
		}
		// Read the markers only from the environment, after the command's
		// own arguments: `grep CLAUDECODE=1` would match itself.
		env := f[1]
		if p.command != "" && strings.HasPrefix(env, p.command) {
			env = env[len(p.command):]
		}
		for _, m := range markerRE.FindAllStringSubmatch(env, -1) {
			if p.env == nil {
				p.env = map[string]string{}
			}
			if _, set := p.env[m[1]]; !set {
				p.env[m[1]] = m[2]
			}
		}
	}
	return s, nil
}

// cwdOf is only needed where lsof is missing, which macOS never is.
func cwdOf(int) string { return "" }

// commandOf is the command line a process runs now, or "" when it is gone.
func commandOf(pid int) string {
	return strings.TrimSpace(ps(nil, "ww", "-p", strconv.Itoa(pid), "-o", "command="))
}
