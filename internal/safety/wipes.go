package safety

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// Wipes: commands that erase a whole machine root or the home folder,
// which are never run.

// wipesAnywhere looks for a wipe in every simple command of the line. A cd
// before it counts: "cd ~ && rm -rf *" erases the home directory without
// naming it.
func wipesAnywhere(cmd string) bool {
	dir := ""
	for _, seg := range shellSegments(cmd) {
		run := afterWrappers(seg)
		if len(run) == 0 {
			continue
		}
		if verb := path.Base(run[0]); verb == "cd" || verb == "pushd" {
			dir = cdTarget(dir, run[1:])
			continue
		}
		if wipesMachine(strings.Join(run, " ")) {
			return true
		}
		if dir != "" && wipesMachine(strings.Join(rebase(dir, run), " ")) {
			return true
		}
		if dir != "" && path.Base(run[0]) == "git" && gitCleanWipes(run[1:], dir) {
			return true
		}
	}
	return false
}

// cdTarget is where "cd args" leaves a shell that was in dir ("" when
// unknown). Without an argument cd goes home.
func cdTarget(dir string, args []string) string {
	to := "~"
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			to = a
			break
		}
	}
	if strings.HasPrefix(to, "/") || strings.HasPrefix(to, "~") || strings.HasPrefix(to, "$") {
		return to
	}
	if dir == "" {
		// Relative to where omega itself is.
		wd, err := getwd()
		if err != nil || wd == "" {
			return ""
		}
		dir = strings.ReplaceAll(wd, `\`, "/")
	}
	return dir + "/" + to
}

// rebase rewrites the relative arguments of a command as paths under dir.
// A pattern that takes everything (*, .*, ./*) becomes dir/*.
func rebase(dir string, run []string) []string {
	out := []string{run[0]}
	for _, a := range run[1:] {
		switch {
		case a == "" || strings.HasPrefix(a, "-") || strings.HasPrefix(a, "/") || strings.HasPrefix(a, "~") || strings.HasPrefix(a, "$"):
		case strings.ContainsAny(a, "*?") && strings.Trim(a, "./*?") == "":
			a = dir + "/*"
		default:
			a = dir + "/" + a
		}
		out = append(out, a)
	}
	return out
}

// homeSentinel stands in for ~ and $HOME so path.Clean can see that
// "$HOME/.." is /Users and "~" itself is the home directory.
const homeSentinel = "/Users/omega"

// userHome is the real home directory, "" when unknown. Tests replace it.
var userHome = func() string {
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return ""
	}
	return path.Clean(strings.ReplaceAll(h, `\`, "/"))
}

// wipesMachine reports a command that erases a whole machine root: /, the
// home directory, /Users, /System, /Library or /Applications, and on Linux
// /home, /root, /etc, /usr, /var, /boot, /bin, /sbin, /lib, /lib64 or /opt. Extra flags
// ("rm -rf / --no-preserve-root") and ".." still count. A path underneath
// one of those directories does not.
func wipesMachine(part string) bool {
	words := commandWords(part)
	i := 0
	for i < len(words) {
		w := words[i]
		if strings.Contains(w, "=") && !strings.HasPrefix(w, "/") && !strings.HasPrefix(w, "-") {
			i++
			continue
		}
		switch w {
		case "sudo", "command", "exec", "nohup", "time":
			i++
			for i < len(words) && strings.HasPrefix(words[i], "-") {
				i++
			}
			continue
		}
		break
	}
	if i >= len(words) {
		return false
	}
	args := words[i+1:]
	switch filepath.Base(words[i]) {
	case "rm", "rmdir":
		return rmWipes(args)
	case "find":
		return findWipes(args)
	case "mv":
		return mvWipes(args)
	case "rsync":
		return rsyncWipes(args)
	case "git":
		return gitCleanWipes(args, "")
	}
	return false
}

// mvWipes reports a root moved away: "mv ~ /tmp/x" takes the home
// directory with it as surely as rm. The last word is where things go,
// unless -t names it.
func mvWipes(args []string) bool {
	var paths []string
	target := false
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-t" || a == "--target-directory":
			target = true
			i++
		case strings.HasPrefix(a, "--target-directory="):
			target = true
		case strings.HasPrefix(a, "-"):
		default:
			paths = append(paths, a)
		}
	}
	if !target && len(paths) > 0 {
		paths = paths[:len(paths)-1]
	}
	for _, p := range paths {
		if isRootTarget(p) {
			return true
		}
	}
	return false
}

// rsyncWipes reports rsync deleting in a root: with --delete (or one of
// its variants) everything in the destination that the source lacks goes,
// and the destination is the last word.
func rsyncWipes(args []string) bool {
	deletes := false
	var last string
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "--del") || a == "--remove-source-files":
			deletes = true
		case !strings.HasPrefix(a, "-"):
			last = a
		}
	}
	return deletes && last != "" && !strings.Contains(last, ":") && isRootTarget(last)
}

// gitCleanWipes reports "git clean -f" in a root (git -C ~ clean -fdx, or
// after cd ~): it deletes every file git does not track, which in the home
// directory is everything.
func gitCleanWipes(args []string, dir string) bool {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-C" && i+1 < len(args):
			dir = args[i+1]
			i++
		case has("-c --git-dir --work-tree", a):
			i++
		case strings.HasPrefix(a, "--work-tree="):
			dir = strings.TrimPrefix(a, "--work-tree=")
		case strings.HasPrefix(a, "-"):
		case a == "clean":
			force := false
			for _, b := range args[i+1:] {
				force = force || b == "--force" || (shortFlags.MatchString(b) && strings.Contains(b, "f"))
			}
			return force && dir != "" && isRootTarget(dir)
		default:
			return false
		}
	}
	return false
}

func commandWords(part string) []string {
	raw := strings.Fields(part)
	out := make([]string, len(raw))
	for i, w := range raw {
		out[i] = strings.Trim(w, `"'`)
	}
	return out
}

func rmWipes(args []string) bool {
	flagsDone := false
	for _, a := range args {
		if a == "--" {
			flagsDone = true
			continue
		}
		if !flagsDone && strings.HasPrefix(a, "-") {
			continue
		}
		if isRootTarget(a) {
			return true
		}
	}
	return false
}

func findWipes(args []string) bool {
	if !findChanges(args) {
		return false
	}
	for _, a := range args {
		if isRootTarget(a) {
			return true
		}
	}
	return false
}

// findRuns are the programs find may run on what it finds and stay
// read-only: they print names, sizes and types.
var findRuns = map[string]bool{"ls": true, "stat": true, "file": true, "wc": true, "du": true, "head": true, "basename": true, "echo": true}

// findChanges reports a find that deletes, writes a file, asks, or runs
// anything but one of findRuns.
func findChanges(args []string) bool {
	for i, a := range args {
		switch a {
		case "-delete", "-ok", "-okdir", "-fprint", "-fprint0", "-fprintf", "-fls":
			return true
		case "-exec", "-execdir":
			if i+1 >= len(args) || !findRuns[args[i+1]] {
				return true
			}
		}
	}
	return false
}

var (
	homeVar = regexp.MustCompile(`\$\{HOME[^}]*\}|\$HOME\b|/(Users|home)/\$\{?(USER|LOGNAME)\}?`)
	// ${VAR:-word}: the word is what the shell uses when VAR is empty.
	varDefault = regexp.MustCompile(`\$\{\w+:?[-=+]([^}]*)\}`)
)

func isRootTarget(arg string) bool {
	if arg == "" || strings.HasPrefix(arg, "-") {
		return false
	}
	if rootPath(arg, homeSentinel) || rootPath(varDefault.ReplaceAllString(arg, "$1"), homeSentinel) {
		return true
	}
	// "~/../name" and the home directory written out in full are only
	// the home directory for the real one.
	if home := userHome(); home != "" && home != "/" {
		return rootPath(arg, home)
	}
	return false
}

// rootPath reports arg, with ~ and $HOME read as home, naming a machine
// root or home itself.
func rootPath(arg, home string) bool {
	// The working directory itself: rm -rf "$PWD" empties wherever the
	// command happens to run, which nobody can see in the command.
	if pwdVar.MatchString(arg) {
		return true
	}
	expanded := homeVar.ReplaceAllString(arg, home)
	if strings.HasPrefix(expanded, "~") {
		rest := strings.TrimPrefix(expanded, "~")
		if rest != "" && !strings.HasPrefix(rest, "/") {
			// ~name is that user's home directory.
			name, sub, _ := strings.Cut(rest, "/")
			return sub == "" || sub == "*" || (strings.ContainsAny(name, "*?[") && rootPath("/Users/"+name+"/"+sub, home))
		}
		expanded = home + rest
	}
	if !strings.HasPrefix(expanded, "/") {
		return false
	}
	if strings.HasSuffix(expanded, "/*") {
		expanded = strings.TrimSuffix(expanded, "/*")
		if expanded == "" {
			expanded = "/"
		}
	}
	p := path.Clean(expanded)
	if machineRoots[p] || p == home {
		return true
	}
	// A pattern that can match one of them: /Users/l*, /U*, /home/*. Another
	// user's home named in full is left to approval (a removed account's
	// leftovers); a pattern there can also reach this user's own.
	if strings.ContainsAny(p, "*?[") {
		parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
		switch len(parts) {
		case 1:
			for root := range machineRoots {
				if ok, _ := path.Match(p, root); ok {
					return true
				}
			}
		case 2:
			for _, top := range []string{"Users", "home"} {
				if ok, _ := path.Match(parts[0], top); ok {
					return true
				}
			}
		}
	}
	return false
}

var (
	machineRoots = map[string]bool{"/": true, "/Users": true, "/System": true, "/Library": true, "/Applications": true,
		"/home": true, "/root": true, "/etc": true, "/usr": true, "/var": true, "/boot": true, "/bin": true, "/sbin": true,
		"/lib": true, "/lib64": true, "/opt": true}
	pwdVar = regexp.MustCompile(`^"?\$\{?(PWD|OLDPWD)\}?"?(/\*?|/\.)?$`)
)
