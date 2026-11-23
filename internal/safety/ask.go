package safety

import (
	"path"
	"regexp"
	"strings"
)

// The commands below are not forbidden, but a "yes to everything" must not
// cover them either: each one deletes what cannot be fetched again, changes
// the user's own configuration for good, runs code nobody can read in the
// command, or hands a credential to whoever reads the output. AlwaysConfirm
// reports them, so the agent asks every time and skips them when nobody is
// there to answer.

// alwaysAsks reports one of those commands anywhere in the line.
func alwaysAsks(command string) bool {
	if pipesIntoInterpreter(command) {
		return true
	}
	for _, seg := range shellSegments(command) {
		if sudoAsks(seg) {
			return true
		}
		run := afterWrappers(seg)
		if len(run) == 0 {
			continue
		}
		verb := strings.TrimSuffix(strings.ToLower(path.Base(run[0])), ".exe")
		args := run[1:]
		switch verb {
		case "rm":
			if rmRecursive(args) && anyArg(rejoin(args), underHome) {
				return true
			}
		case "git":
			if gitAsks(args) {
				return true
			}
		case "go":
			if len(args) > 0 && args[0] == "env" && anyArg(args[1:], func(a string) bool { return a == "-w" || a == "-u" }) {
				return true
			}
		case "defaults":
			if sub := subcommand(args); has("write delete import rename", sub) {
				return true
			}
		case "brew":
			if brewForces(args) {
				return true
			}
		case "mo":
			if moConfirms(args) {
				return true
			}
		case "docker", "podman", "nerdctl":
			if containerAsks(args) {
				return true
			}
		case "sh", "bash", "zsh", "dash", "ksh", "fish", "source", ".":
			// afterWrappers skips a shell; it gets here only as "source" or "."
			if positional(args) > 0 {
				return true
			}
		case "python", "python3", "python2", "node", "nodejs", "perl", "ruby", "php", "osascript", "pwsh", "powershell":
			if runsOpaque(verb, args) {
				return true
			}
		case "gh":
			// gh auth token, and gh auth status --show-token, print the token.
			if len(args) > 1 && args[0] == "auth" && (args[1] == "token" || anyArg(args[2:], func(a string) bool { return a == "--show-token" || a == "-t" })) {
				return true
			}
		case "gcloud":
			if anyArg(args, func(a string) bool {
				return strings.HasPrefix(a, "print-access-token") || strings.HasPrefix(a, "print-identity-token")
			}) {
				return true
			}
		case "az":
			if strings.Contains(strings.Join(args, " "), "get-access-token") {
				return true
			}
		case "security":
			// -i reads commands from standard input; the find-* with -w or -g
			// are refused outright by leaksSecret.
			if anyArg(args, func(a string) bool { return a == "-i" || a == "-p" }) || securityReveals(seg) {
				return true
			}
		}
		// "sh script" and "bash -x script": a shell given a file runs
		// whatever is in the file. afterWrappers skips the shell itself, so
		// look at the segment as it was typed.
		if shellRunsFile(seg) {
			return true
		}
	}
	return false
}

func anyArg(args []string, f func(string) bool) bool {
	for _, a := range args {
		if f(a) {
			return true
		}
	}
	return false
}

// positional counts the arguments that are not options.
func positional(args []string) int {
	n := 0
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			n++
		}
	}
	return n
}

// sudoAsks reports a command run under sudo or doas. Checking or dropping
// the cached credentials (sudo -n true, -v, -k, -l) runs nothing as root.
func sudoAsks(seg []string) bool {
	for i, w := range seg {
		base := path.Base(w)
		if base != "sudo" && base != "doas" {
			// Only in command position: the words before it are wrappers or
			// assignments ("env X=1 sudo …", "nice sudo …").
			if strings.Contains(w, "=") || has("env nice nohup time command exec builtin if then else elif do while until !", base) || strings.HasPrefix(w, "-") {
				continue
			}
			return false
		}
		rest := seg[i+1:]
		for _, a := range rest {
			if !has("-n -v -k -K -l -nv -vn -nk -kn -nl --non-interactive --validate --reset-timestamp --remove-timestamp --list true", a) {
				return true
			}
		}
		return false
	}
	return false
}

// rejoin puts back together the paths with spaces that shellSegments cut
// apart ("~/Library/Application Support/x", iOS\ DeviceSupport): a word
// that does not start a path or an option continues the one before it.
func rejoin(args []string) []string {
	var out []string
	for _, a := range args {
		if n := len(out); n > 0 && a != "" && !strings.HasPrefix(a, "-") && !strings.HasPrefix(out[n-1], "-") &&
			strings.IndexByte("/~$.", a[0]) < 0 {
			out[n-1] += " " + a
			continue
		}
		out = append(out, a)
	}
	return out
}

// rmRecursive reports rm -r, -R, -rf, -fr, --recursive.
func rmRecursive(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "--recursive" || (shortFlags.MatchString(a) && strings.ContainsAny(a, "rR")) {
			return true
		}
	}
	return false
}

// tempDirs are where deleting is routine: nothing there is the user's.
var tempDirs = regexp.MustCompile(`^(\$\{?TMPDIR\}?|/tmp|/private/tmp|/var/folders|/private/var/folders)(/|$)`)

// homePrefix is a path written from the home directory: ~, ~user, $HOME,
// ${HOME}, /Users/name, /home/name.
var homePrefix = regexp.MustCompile(`^(~[\w.-]*|\$\{?HOME\}?|/Users/[^/]+|/home/[^/]+)(/|$)`)

// regenerable are the folders under the home directory whose contents
// come back by themselves (caches, logs, build products, the Trash): a
// deletion there is an ordinary dangerous command. Spotify is the
// exception inside Caches: it keeps the offline downloads there.
var regenerable = regexp.MustCompile(`^(\.Trash|Library/Caches|\.cache|Library/Logs|Library/Developer/Xcode/DerivedData|` +
	`Library/Developer/Xcode/iOS DeviceSupport|Library/Developer/CoreSimulator/Caches|\.gradle/caches|\.gradle/daemon|\.npm/_cacache)/.`)

var keptInCaches = regexp.MustCompile(`(?i)^Library/Caches/com\.spotify\.client(/|$)`)

// underHome reports a path in the user's own files: projects, documents,
// app data in ~/Library. Deleting there goes through the user every time.
// Temporary folders and the regenerable ones are left out.
func underHome(a string) bool {
	if a == "" || strings.HasPrefix(a, "-") || tempDirs.MatchString(a) {
		return false
	}
	rest, ok := "", false
	if m := homePrefix.FindString(a); m != "" {
		rest, ok = strings.TrimPrefix(a, m), true
	} else if h := userHome(); h != "" && h != "/" && (a == h || strings.HasPrefix(a, h+"/")) {
		rest, ok = strings.TrimPrefix(strings.TrimPrefix(a, h), "/"), true
	}
	if !ok {
		return false
	}
	rest = path.Clean("/" + rest)[1:]
	return keptInCaches.MatchString(rest) || !regenerable.MatchString(rest)
}

// gitGlobals are git's options before the subcommand that take a value.
const gitGlobals = "-C -c --git-dir --work-tree --namespace --exec-path --config-env"

// gitSplit returns git's subcommand and its arguments, past the global
// options: git -C repo worktree remove x.
func gitSplit(args []string) (string, []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			return a, args[i+1:]
		}
		if has(gitGlobals, a) {
			i++
		}
	}
	return "", nil
}

// gitAsks reports git worktree remove, which deletes a checkout and any
// change in it that was never committed, and a write to the user's global
// or system git configuration.
func gitAsks(args []string) bool {
	sub, rest := gitSplit(args)
	switch sub {
	case "worktree":
		return len(rest) > 0 && (rest[0] == "remove" || rest[0] == "rm")
	case "config":
		return gitConfigWritesGlobal(rest)
	}
	return false
}

// gitConfigWritesGlobal reports git config --global (or --system) that
// changes something. Reading a key, listing, or --get* does not.
func gitConfigWritesGlobal(args []string) bool {
	global := false
	for _, a := range args {
		if a == "--global" || a == "--system" {
			global = true
		}
	}
	if !global {
		return false
	}
	// The subcommand form of git 2.46: git config set|unset|edit --global.
	if sub := subcommand(args); has("set unset edit rename-section remove-section", sub) {
		return true
	} else if has("get list", sub) {
		return false
	}
	reads, n := false, 0
	for _, a := range args {
		name, _, _ := strings.Cut(a, "=")
		switch {
		case has("--unset --unset-all --add --replace-all --remove-section --rename-section --edit -e", name):
			return true
		case strings.HasPrefix(a, "--get") || has("--list -l --show-origin --show-scope --name-only --null -z", name):
			reads = true
		case !strings.HasPrefix(a, "-"):
			n++
		}
	}
	// "git config --global name value" sets; the name alone prints it.
	return !reads && n >= 2
}

// brewForces reports brew install, reinstall, uninstall or upgrade with
// --force (or -f), and uninstall --zap, which deletes the app's data too.
func brewForces(args []string) bool {
	if !has("install reinstall uninstall remove rm upgrade link unlink", subcommand(args)) {
		return false
	}
	for _, a := range args {
		if a == "--force" || a == "--zap" || a == "--force-bottle" || (shortFlags.MatchString(a) && strings.Contains(a, "f")) {
			return true
		}
	}
	return false
}

// moConfirms reports a Mole command that deletes without the tool's own
// question: mo purge (build folders across the projects), and any mo
// command told --yes, -y or --force. Adding --yes after the tool refused is
// the user's answer, not the agent's.
func moConfirms(args []string) bool {
	for _, a := range args {
		if a == "--dry-run" || a == "-n" {
			return false
		}
	}
	if subcommand(args) == "purge" {
		return true
	}
	return anyArg(args, func(a string) bool { return a == "--yes" || a == "-y" || a == "--force" || a == "-f" })
}

// containerAsks reports what dockerDestructive does not: a reset that
// wipes all of podman's storage, removing a running service, secret or
// config, and a container started with the home directory or the whole
// disk mounted into it, which can then read and change any file.
func containerAsks(args []string) bool {
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		if has(containerGlobals, args[i]) {
			i++
		}
		i++
	}
	if i >= len(args) {
		return false
	}
	sub, rest := args[i], args[i+1:]
	next := ""
	if len(rest) > 0 {
		next = rest[0]
	}
	switch sub {
	case "system", "machine":
		return next == "reset"
	case "service", "secret", "config", "node", "plugin":
		return next == "rm" || next == "remove"
	case "run", "create":
		return mountsHome(rest)
	case "container":
		return (next == "run" || next == "create") && mountsHome(rest[1:])
	case "swarm":
		return next == "leave"
	}
	return false
}

// mountsHome reports -v, --volume or --mount whose source is the home
// directory, a folder above it, or /, and --privileged, which gives the
// container the host's devices.
func mountsHome(args []string) bool {
	for i, a := range args {
		var src string
		switch {
		case a == "--privileged" || a == "--privileged=true":
			return true
		case (a == "-v" || a == "--volume" || a == "--mount") && i+1 < len(args):
			src = args[i+1]
		case strings.HasPrefix(a, "--volume="), strings.HasPrefix(a, "--mount="):
			_, src, _ = strings.Cut(a, "=")
		case strings.HasPrefix(a, "-v") && len(a) > 2 && !strings.HasPrefix(a, "--"):
			src = a[2:]
		default:
			continue
		}
		if strings.Contains(src, "source=") || strings.Contains(src, "src=") {
			for _, kv := range strings.Split(src, ",") {
				if k, v, ok := strings.Cut(kv, "="); ok && (k == "source" || k == "src") {
					src = v
				}
			}
		} else {
			src, _, _ = strings.Cut(src, ":")
		}
		if src == "/" || isRootTarget(src) || isRootTarget(strings.TrimSuffix(src, "/")) {
			return true
		}
	}
	return false
}

var interpreters = regexp.MustCompile(`^(sh|bash|zsh|dash|ksh|fish|csh|tcsh|python[0-9.]*|node|nodejs|perl|ruby|php|osascript|pwsh|powershell|iex|invoke-expression|source|\.)(\.exe)?$`)

// pipesIntoInterpreter reports a pipeline that feeds a shell or an
// interpreter on its standard input: "… | base64 -d | sh". What runs is
// whatever the earlier stages print, which the command does not show.
func pipesIntoInterpreter(command string) bool {
	for _, pipeline := range splitUnquoted(stripComments(command), false, true) {
		stages := splitUnquoted(pipeline, true, false)
		for _, stage := range stages[1:] {
			words := strings.Fields(dequote(stage))
			for len(words) > 0 {
				w := strings.ToLower(path.Base(words[0]))
				if strings.Contains(words[0], "=") || strings.HasPrefix(words[0], "-") || has("sudo doas env exec command nice nohup time xargs", w) {
					words = words[1:]
					continue
				}
				break
			}
			if len(words) > 0 && interpreters.MatchString(strings.ToLower(path.Base(words[0]))) {
				return true
			}
		}
	}
	return false
}

// shellRunsFile reports a shell started on a script file: "sh install.sh",
// "bash -x ./x". -c runs the text that follows, which the policy reads.
func shellRunsFile(seg []string) bool {
	for i, w := range seg {
		base := strings.TrimSuffix(path.Base(w), ".exe")
		if strings.Contains(w, "=") || has("sudo doas env exec command nice nohup time", base) {
			continue
		}
		if !has("sh bash zsh dash ksh fish csh tcsh", base) {
			return false
		}
		for _, a := range seg[i+1:] {
			if a == "-c" || (shortFlags.MatchString(a) && strings.Contains(a, "c")) {
				return false
			}
			if !strings.HasPrefix(a, "-") {
				return true
			}
		}
		return false
	}
	return false
}

// runsOpaque reports an interpreter handed code in the command line
// (python3 -c, node -e, perl -e, ruby -e, php -r) or a script file: the
// code is not shell, so nothing in the policy reads it. osascript counts
// when it can reach the shell ("do shell script") or runs a file.
func runsOpaque(verb string, args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch verb {
	case "osascript":
		joined := strings.ToLower(strings.Join(args, " "))
		if strings.Contains(joined, "do shell script") {
			return true
		}
		// The words after -e are the script itself (the quotes are gone
		// here); without -e the first word is a script file.
		if anyArg(args, func(a string) bool { return a == "-e" }) {
			return false
		}
		return positional(args) > 0
	case "pwsh", "powershell":
		return anyArg(args, func(a string) bool {
			l := strings.ToLower(a)
			return l == "-encodedcommand" || l == "-enc" || l == "-e" || l == "-ec" || l == "-file" || l == "-f"
		})
	}
	for _, a := range args {
		switch {
		case has("-c -e -E -r -p --eval --print --exec -x", a), strings.HasPrefix(a, "--eval="):
			return true
		case a == "-m" || has("--version -V --help -h -v", a):
			return false // a module by name (python3 -m pip), or a question
		case !strings.HasPrefix(a, "-"):
			return true // a script file
		}
	}
	return false
}
