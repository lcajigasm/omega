package aiagents

import "strings"

// Some programs an agent's command starts become daemons the whole machine
// uses: git's file-system monitor, mutagen's sync daemon, 1Password's op
// daemon, ssh and gpg agents. They are listed, since an agent started them,
// but ending everything a session left, or the session itself, keeps them:
// ending one stops syncs, signs a CLI out or breaks other terminals. They
// can still be ended one by one, by PID.
var sharedDaemons = []struct {
	name string
	arg  string // a word the command line must hold; "" for any
}{
	{"git", "fsmonitor--daemon"},
	{"mutagen", "daemon"},
	{"op", "daemon"},
	{"gpg-agent", ""},
	{"ssh-agent", ""},
	{"watchman", ""},
	{"keybase", ""},
	{"syncthing", ""},
	{"ollama", "serve"},
	{"adb", "fork-server"},
	{"colima", ""},
	{"limactl", ""},
	{"com.docker.backend", ""},
}

// shared reports whether a process is one of those daemons.
func shared(p Proc) bool {
	name := p.Name()
	words := strings.Fields(p.Command)
	for _, d := range sharedDaemons {
		if name != d.name {
			continue
		}
		if d.arg == "" {
			return true
		}
		for _, w := range words[min(1, len(words)):] {
			if w == d.arg {
				return true
			}
		}
	}
	return false
}
