// Package safety classifies shell commands by risk so the agent knows which
// ones it may run on its own, which need the user's approval, and which it
// must never run.
package safety

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// Risk levels, ordered from harmless to forbidden.
type Risk int

const (
	Safe      Risk = iota // read-only
	Normal                // routine updates of user-installed software
	Dangerous             // deletes data, needs sudo, changes config or system state
	Blocked               // never run
)

func (r Risk) String() string {
	return [...]string{"read-only", "normal", "dangerous", "blocked"}[r]
}

var blocked = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bdiskutil\s+(erase\w*|partition\w*|zero\w*|random\w*|secureErase|reformat|resetFusion|(apfs|ap|cs|corestorage|ar|appleraid)\s+(delete\w*|erase\w*))`),
	regexp.MustCompile(`\bdd\b.*\bof=/dev/`),
	regexp.MustCompile(`>\s*/dev/(r?disk|sd)`),
	// Copying onto a whole disk is dd by another name.
	regexp.MustCompile(`\bcp\b[^;|&]*\s/dev/` + wholeDisk + `\S*\s*($|[;|&])`),
	regexp.MustCompile(`\btee\b[^;|&]*\s/dev/` + wholeDisk),
	regexp.MustCompile(`\bcsrutil\s+(disable|clear|authenticated-root)`),
	regexp.MustCompile(`\bspctl\s+--master-disable`),
	regexp.MustCompile(`:\(\)\s*\{`),
	regexp.MustCompile(`\b(curl|wget)\b[^|]*\|\s*(sudo\s+)?(ba|z|)sh\b`),
	regexp.MustCompile(`\bchmod\s+(-R\s+)?[0-7]*777\s+/(\s|$)`),
	// Deleting backups and switching Time Machine off. Thinning the local
	// snapshots (deletelocalsnapshots) frees space and only needs approval.
	regexp.MustCompile(`\btmutil\s+(delete\b|deleteinprogress|disable)`),
	// Linux: wiping whole disks and switching off rm's own safety net.
	regexp.MustCompile(`>\s*/dev/(nvme|vd|mmcblk|hd)`),
	regexp.MustCompile(`--no-preserve-root`),
	// The helper that answers sudo with the password omega holds for this
	// run, and the folder it lives in. Only sudo calls it: a command that
	// names it wants the password.
	regexp.MustCompile(`(?i)sudo_askpass|askpass\.sock|omega_askpass_sock|omega-sudo-`),
	// The privacy database is only ever changed through tccutil: a direct
	// write can grant a permission the user never approved.
	regexp.MustCompile(`(?i)TCC\.db.*\b(insert|update|delete|replace|drop|alter)\b|\b(insert|update|delete|replace|drop|alter)\b.*TCC\.db`),
	// What was last copied is often a password.
	regexp.MustCompile(`(?i)\b(get-clipboard|gcb|pbpaste|wl-paste)\b|\bxclip\b.*\s-o\b|\bxsel\b.*\s(-o|--output)\b`),
}

const wholeDisk = `(r?disk[0-9]|sd[a-z]|nvme[0-9]|vd[a-z]|mmcblk[0-9]|hd[a-z])`

// blockedTools format disks, erase firmware variables or turn disk
// encryption off. They are matched by name wherever one can run; the name
// alone is not the tool, so "man mkfs" and "ls /usr/sbin | grep nvram" pass.
var blockedTools = regexp.MustCompile(`\b(mkfs|newfs(_\w+)?|wipefs|blkdiscard|nvram|fdesetup)\b`)

// printsOnly are programs that only show the words they are given, or look
// them up.
var printsOnly = map[string]bool{"man": true, "ls": true, "grep": true, "egrep": true, "fgrep": true, "rg": true,
	"echo": true, "printf": true, "which": true, "type": true, "whereis": true, "whatis": true, "apropos": true,
	"cat": true, "head": true, "tail": true, "less": true, "more": true, "file": true, "stat": true, "tldr": true}

// runsText are programs that run the text they are handed: with one of
// them on the line, a name that is only printed can still end up running.
var runsText = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "fish": true,
	"eval": true, "xargs": true, "source": true, ".": true, "parallel": true}

const (
	nvramReads    = "-p -x -xp -px"
	fdesetupReads = "status isactive haspersonalrecoverykey hasinstitutionalrecoverykey supportsauthrestart help version"
)

// runsBlockedTool reports a command that runs one of the blockedTools,
// other than to read: "nvram -p" and "fdesetup status" change nothing.
func runsBlockedTool(cmd string) bool {
	if !blockedTools.MatchString(cmd) {
		return false
	}
	segs := shellSegments(cmd)
	for _, seg := range segs {
		for _, w := range seg {
			if runsText[path.Base(w)] {
				return true
			}
		}
	}
	for _, seg := range segs {
		if !blockedTools.MatchString(strings.Join(seg, " ")) {
			continue
		}
		run := afterWrappers(seg)
		if len(run) == 0 {
			return true
		}
		verb := path.Base(run[0])
		switch {
		case seg[0] == "command" && len(seg) > 1 && (seg[1] == "-v" || seg[1] == "-V"):
		case printsOnly[verb]:
		case verb == "find" && !findChanges(run[1:]):
		case verb == "nvram" && len(run) > 1 && allIn(nvramReads, run[1:]):
		case verb == "fdesetup" && len(run) == 2 && has(fdesetupReads, run[1]):
		default:
			return true
		}
	}
	return false
}

func allIn(list string, words []string) bool {
	for _, w := range words {
		if !has(list, w) {
			return false
		}
	}
	return true
}

// Read-only command prefixes.
var safePrefixes = []string{
	"ls", "cat", "head", "tail", "wc", "grep", "rg", "file", "stat", "which", "type", "command -v",
	"echo", "printf", "pwd", "whoami", "id", "date", "uptime", "uname", "sw_vers", "hostname",
	"df", "du", "vm_stat", "sysctl", "ps", "top -l", "pmset -g", "ioreg", "system_profiler",
	"cut", "tr", "basename", "dirname", "readlink", "realpath", "diff", "shasum", "md5", "md5sum", "sha256sum",
	"pgrep", "mdls", "log show",
	"defaults read", "defaults read-type", "defaults domains", "defaults find",
	"launchctl list", "launchctl print",
	"diskutil list", "diskutil info", "diskutil apfs list",
	"tmutil listlocalsnapshots", "csrutil status", "find", "sort", "uniq", "jq",
	"brew list", "brew ls", "brew info", "brew outdated", "brew doctor", "brew config", "brew --version", "brew --prefix",
	"brew deps", "brew uses", "brew desc", "brew missing", "brew leaves", "brew tap", "brew search", "brew services list",
	"brew cleanup -n", "brew cleanup --dry-run",
	"brew autoremove -n", "brew autoremove --dry-run",
	"mas list", "mas outdated", "npm ls", "npm list", "npm outdated", "npm config get", "npm doctor",
	"npm root", "npm prefix", "npm view",
	"pnpm ls", "pnpm outdated", "pip list", "pip3 list", "pip show", "pip3 show", "pip cache info", "pip3 cache info",
	"pipx list", "uv tool list", "uv cache dir", "cargo install --list", "rustup show", "rustup check",
	"gem list", "gh extension list", "go version", "go env", "node --version", "python3 --version",
	"docker system df", "docker ps", "docker images", "docker image ls", "docker container ls", "docker volume ls",
	"docker info", "docker version", "docker builder du", "docker buildx du", "docker inspect", "docker logs", "docker stats",
	"git status", "git log", "git diff", "git show", "git --version", "git worktree list", "git branch", "git rev-parse",
	"git stash list", "ollama list", "ollama ps", "lms ls", "lms ps", "tmutil listbackups", "mdfind", "sw_vers",
	"pmset -g batt", "brew services info", "npm config list", "pip3 config list",
	"env", "printenv", "git config --list", "git config --global --list", "git config --get", "git config --global --get",
	"git config -l", "pnpm store path", "pnpm config list", "npm config get", "pipx environment", "pipx --version",
	"uv cache dir", "uv python list", "uv --version", "rustup toolchain list", "go env", "brew --cache", "brew --repo",
	"time zsh -i -c exit", "docker context ls", "launchctl list", "tmutil destinationinfo",
	"topgrade --dry-run", "topgrade -n", "topgrade --version", "topgrade --config-reference",
	"mo --version", "mo clean --dry-run", "mo optimize --dry-run", "mo uninstall --dry-run", "mo history",
	"mo --help", "mo help",
	// Linux.
	"test", "journalctl", "coredumpctl list", "coredumpctl info", "zypper --non-interactive list-updates",
	"free", "lsb_release", "lscpu", "lsblk", "findmnt", "nproc",
	// systemctl, systemd-analyze, loginctl, hostnamectl, rpm, dpkg, pacman,
	// paccache, apt-get and dnf history are decided by queryTool: each also
	// has subcommands and flags that change the system. So are the tools
	// with a read-only form that takes more words after it (git remote,
	// softwareupdate, mdutil, xattr…).
	"apt list", "apt show", "apt policy", "apt search", "apt-cache show", "apt-cache policy", "apt-cache search",
	"apt-cache depends", "apt-cache rdepends", "apt-cache madison", "apt-cache pkgnames", "apt-cache stats",
	"apt-mark showmanual", "apt-mark showhold", "apt-mark showauto", "dpkg-query",
	"dnf check-update", "dnf list", "dnf info", "dnf repolist", "dnf repoquery", "dnf search",
	"checkupdates",
	"zypper list-updates", "zypper lu", "zypper list-patches", "zypper lp", "zypper search", "zypper se",
	"zypper info", "zypper repos", "zypper lr", "zypper packages",
	"flatpak list", "flatpak remote-ls", "flatpak remotes", "flatpak info", "flatpak history", "flatpak --version",
	"snap list", "snap refresh --list", "snap info", "snap version", "snap changes", "snap find",
	"fwupdmgr get-devices", "fwupdmgr get-updates",
}

// Routine, recoverable update operations.
var normalPrefixes = []string{
	"brew update", "brew upgrade", "brew fetch", "mas upgrade",
	"npm update -g", "npm update --global", "pnpm update -g", "pnpm up -g", "bun upgrade",
	"pipx upgrade", "pipx upgrade-all", "uv tool upgrade", "rustup update", "cargo install-update",
	"gh extension upgrade", "mo update", "gem update --user-install", "tldr --update",
	// Flatpak updates need no root: polkit lets the logged-in user do them.
	"flatpak update",
}

var (
	sedInPlace = regexp.MustCompile(`\bsed\b.*\s-i`)
	// journalctl only reads, except when it trims, rotates or rewrites the journal.
	journalWrites = regexp.MustCompile(`\s--(vacuum-\w+|rotate|flush|sync|relinquish-var|smart-relinquish-var|setup-keys|update-catalog)\b`)
	// 2>&1 and >&- write no file. The number must end there: in ">&2file"
	// the shell reads ">&2" and then a file name.
	fdDup           = regexp.MustCompile(`[0-9]*>&(?:[0-9]+|-)(\s|[;|&)]|$)`)
	devNullRedirect = regexp.MustCompile(`[0-9&]?>{1,2}\s*/dev/null(\s|[;|&)]|$)`)
	// $((1+2)) with nothing but numbers in it runs nothing.
	arithmetic = regexp.MustCompile(`\$\(\([0-9+\-*/%\s()]*\)\)`)
)

var dockerDestructive = regexp.MustCompile(`\b(docker|podman|nerdctl)\s+((system|image|container|volume|network|builder|buildx|pod)\s+prune|rmi|rm|volume\s+(rm|remove)|image\s+(rm|remove)|container\s+(rm|remove)|buildx\s+rm|stack\s+(rm|remove))\b`)

// containerGlobals are the options docker, podman and nerdctl take before
// the subcommand with a value after them.
const containerGlobals = "--context -c -H --host --config -l --log-level --tlscacert --tlscert --tlskey " +
	"--connection --url --identity --root --runroot --storage-driver --storage-opt --namespace -n --address -a " +
	"--cgroup-manager --data-root --snapshotter --hosts-dir --cni-path --tmpdir --runtime"

// AlwaysConfirm reports commands that must be confirmed by the user every
// time, even under --yes: Docker cleanups can delete images, volumes and
// databases that are expensive or impossible to recreate.
func AlwaysConfirm(command string) bool {
	if dockerDestructive.MatchString(command) {
		return true
	}
	for _, seg := range shellSegments(command) {
		run := afterWrappers(seg)
		if len(run) == 0 {
			continue
		}
		switch path.Base(run[0]) {
		case "docker", "podman", "nerdctl":
			if containerDeletes(run[1:]) {
				return true
			}
		case "docker-compose", "podman-compose":
			if composeDeletes(run[1:]) {
				return true
			}
		}
	}
	return false
}

// containerDeletes reads the arguments of docker (podman, nerdctl) past
// the global options, which may sit between the program and the subcommand:
// docker --context default system prune.
func containerDeletes(args []string) bool {
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
	sub, next := args[i], ""
	if i+1 < len(args) {
		next = args[i+1]
	}
	switch sub {
	case "rm", "rmi":
		return true
	case "system", "image", "container", "volume", "network", "builder", "buildx", "pod", "stack", "machine":
		return next == "prune" || next == "rm" || next == "remove"
	case "compose":
		return composeDeletes(args[i+1:])
	}
	return false
}

// composeDeletes reports "compose down" with the volumes or the images,
// and "compose rm".
func composeDeletes(args []string) bool {
	down := false
	for _, a := range args {
		down = down || a == "down"
		if a == "rm" {
			return true
		}
	}
	if !down {
		return false
	}
	for _, a := range args {
		if a == "--volumes" || strings.HasPrefix(a, "--rmi") || (shortFlags.MatchString(a) && strings.Contains(a, "v")) {
			return true
		}
	}
	return false
}

// Classify returns the highest risk among the parts of a (possibly compound) command.
func Classify(command string) Risk {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return Safe
	}
	for _, re := range blocked {
		if re.MatchString(cmd) {
			return Blocked
		}
	}
	if runsBlockedTool(cmd) {
		return Blocked
	}
	// Secrets and wipes are looked for in the whole command, through
	// wrappers, quoting and command substitution, before anything else.
	if leaksSecret(cmd) || namesKeptSecret(cmd) {
		return Blocked
	}
	if wipesAnywhere(cmd) || windowsPipeWipes(cmd) {
		return Blocked
	}
	for _, part := range splitCommands(cmd) {
		if windowsWipes(part) {
			return Blocked
		}
	}
	if hidesCommands(cmd) {
		return Dangerous
	}
	// From here the command is read as the shell reads it: without comments.
	code := arithmetic.ReplaceAllString(stripComments(cmd), "0")
	if strings.Contains(code, "$(") || strings.Contains(code, "`") {
		return Dangerous
	}
	// Any parenthesis outside quotes: a subshell, <(cmd), zsh's =(cmd), the
	// glob qualifier *(e:'cmd':), or a PowerShell expression (Remove-Item x).
	bare := unquoted(code)
	if strings.Contains(bare, "(") {
		return Dangerous
	}
	// Any output redirection other than fd duplication or /dev/null writes
	// a file. A > inside quotes is text: echo "a -> b".
	if strings.Contains(fdDup.ReplaceAllString(devNullRedirect.ReplaceAllString(bare, "$1"), "$1"), ">") {
		return Dangerous
	}
	max := Safe
	for _, part := range splitCommands(code) {
		r := classifyOne(part)
		if r > max {
			max = r
		}
	}
	if max == Safe && envDumpReshaped(code) {
		return Dangerous
	}
	return max
}

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

// envDumpReshaped reports "env" or "printenv" printing every variable into
// something that can cut the values up. The credentials tools need stay in
// the environment (see KeptSecretVars) and are masked in the output by
// value: "env | grep -o ." prints them one letter per line, where no mask
// can match. Filters that keep whole lines are fine.
func envDumpReshaped(code string) bool {
	for _, pipeline := range splitUnquoted(code, false, true) {
		dumped := false
		for _, stage := range splitUnquoted(pipeline, true, false) {
			f := strings.Fields(joinText(shellWords(stage)))
			if len(f) == 0 {
				continue
			}
			if !dumped {
				dumped = dumpsEnv(f)
				continue
			}
			switch f[0] {
			case "sort", "uniq", "wc":
			case "grep", "egrep", "fgrep":
				for _, a := range f[1:] {
					if a == "--only-matching" || (shortFlags.MatchString(a) && strings.Contains(a, "o")) {
						return true
					}
				}
			case "head", "tail":
				for _, a := range f[1:] {
					if strings.HasPrefix(a, "--bytes") || (strings.HasPrefix(a, "-") && strings.Contains(a, "c")) {
						return true
					}
				}
			default:
				return true
			}
		}
	}
	return false
}

// dumpsEnv reports env or printenv with no variable named and no command.
func dumpsEnv(f []string) bool {
	if f[0] == "printenv" {
		return len(f) == 1
	}
	if f[0] != "env" {
		return false
	}
	for _, a := range f[1:] {
		if !strings.HasPrefix(a, "-") && !strings.Contains(a, "=") {
			return false
		}
	}
	return true
}

// escapedBreak is a backslash before a character that ends a command or
// opens a quote. The POSIX shells and PowerShell read it in opposite ways
// (in sh, \; is a literal semicolon and \" a literal quote; in PowerShell
// the backslash is an ordinary character), so what follows cannot be split
// into commands with any confidence.
var escapedBreak = regexp.MustCompile("\\\\[;|&\"'`\n\r]")

// hidesCommands reports syntax that can carry a command where the splitter
// does not see one:
//
//   - an escaped quote or separator: echo \"; rm -rf x; echo \" is three
//     commands for the shell and one quoted word for a naive reader;
//   - $'…', whose quoting rules differ from '…';
//   - PowerShell's block comment <# … #>, which a POSIX reader does not end
//     where PowerShell does.
func hidesCommands(cmd string) bool {
	return escapedBreak.MatchString(cmd) || strings.Contains(cmd, "$'") || strings.Contains(cmd, "<#") || strings.Contains(cmd, "#>")
}

// unquoted is the command without what is inside its quotes.
func unquoted(cmd string) string {
	var out strings.Builder
	var quote byte
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		default:
			out.WriteByte(c)
		}
	}
	return out.String()
}

// harmlessVars are the variables a command may be given without changing
// what it runs. Any other assignment in front of a command is treated as a
// change: PATH picks another program, GIT_CONFIG_* and PAGER make a
// read-only tool run one.
var harmlessVars = regexp.MustCompile(`^(LANG|LANGUAGE|LC_[A-Z]+|TZ|TERM|COLUMNS|LINES|NO_COLOR|CLICOLOR|CLICOLOR_FORCE|CI|DEBIAN_FRONTEND|HOMEBREW_NO_[A-Z_]+|HOMEBREW_COLOR|NONINTERACTIVE)=[\w.:/@+-]*$`)

// writesByOption reports a tool from the read-only list used in a way that
// writes a file, changes a setting or runs another program. p holds the
// words as the program receives them.
func writesByOption(p string) bool {
	f := strings.Fields(p)
	if len(f) == 0 {
		return false
	}
	args := f[1:]
	opt := func(names ...string) bool {
		for _, a := range args {
			name, _, _ := strings.Cut(a, "=")
			for _, n := range names {
				if name == n {
					return true
				}
			}
		}
		return false
	}
	// short reports a letter in a cluster of one-letter options (-uo).
	short := func(letter string) bool {
		for _, a := range args {
			if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, letter) {
				return true
			}
		}
		return false
	}
	// positional counts the arguments that are not options, nor the value
	// of one of the options named in takeValue ("-f 1").
	positional := func(takeValue string) (n int) {
		for i := 0; i < len(args); i++ {
			switch {
			case has(takeValue, args[i]):
				i++
			case !strings.HasPrefix(args[i], "-"):
				n++
			}
		}
		return n
	}
	switch f[0] {
	case "rg":
		return opt("--pre", "--pre-glob", "--hostname-bin")
	case "sort":
		return short("o") || opt("--output", "--compress-program", "--files0-from")
	case "uniq":
		return positional("-f -s -w") > 1
	case "file":
		return short("C") || opt("--compile")
	case "date":
		return setsClock(args)
	case "hostname":
		return positional("") > 0 || short("F") || opt("--file")
	case "sysctl":
		return short("w") || short("p") || strings.Contains(strings.Join(args, " "), "=")
	case "go":
		return len(args) > 0 && args[0] == "env" && (opt("-w", "-u"))
	case "brew":
		return len(args) > 0 && args[0] == "tap" && positional("") > 1
	case "git":
		if opt("--output", "--ext-diff", "--textconv") {
			return true
		}
		if len(args) > 0 && args[0] == "branch" {
			lists := opt("--list", "-l", "--merged", "--no-merged", "--contains", "--no-contains", "--points-at", "--show-current", "-a", "--all", "-r", "--remotes")
			for _, a := range args[1:] {
				switch {
				case strings.HasPrefix(a, "--"):
					name, _, _ := strings.Cut(a, "=")
					if !has("--list --all --remotes --merged --no-merged --contains --no-contains --points-at --show-current --verbose --sort --format --color --no-color --column --no-column --abbrev --no-abbrev", name) {
						return true
					}
				case strings.HasPrefix(a, "-"):
					if strings.Trim(a, "-alrv") != "" {
						return true
					}
				case !lists:
					return true // a name alone creates a branch
				}
			}
		}
	}
	return false
}

// setsClock reports a date command that is not only printing. On macOS a
// date given without -j is the new time, and so is "-f format date"; on
// Linux -s and --set are. Everything else (-I, -r seconds, -v, -u, +format,
// -d string, and a date to convert under -j) only prints.
func setsClock(args []string) bool {
	converts := false
	for _, a := range args {
		if a == "-d" || strings.HasPrefix(a, "--date") ||
			(shortFlags.MatchString(a) && !strings.HasPrefix(a, "-I") && !strings.HasPrefix(a, "-v") && strings.Contains(a, "j")) {
			converts = true
		}
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--set" || strings.HasPrefix(a, "--set="):
			return true
		case a == "--date" || a == "--reference" || a == "-d" || a == "-r" || a == "-v" || a == "-z":
			i++ // its value
		case a == "-f":
			if !converts {
				return true
			}
			i += 2 // the format and the date written in it
		case strings.HasPrefix(a, "--"), strings.HasPrefix(a, "-I"), strings.HasPrefix(a, "-v"), strings.HasPrefix(a, "+"):
		case strings.HasPrefix(a, "-") && len(a) > 1:
			// -s sets the clock; f in a cluster hides the two values after it.
			if strings.ContainsAny(a, "sf") {
				return true
			}
			if strings.HasSuffix(a, "r") || strings.HasSuffix(a, "d") {
				i++
			}
		default:
			if !converts {
				return true
			}
		}
	}
	return false
}

// dryRun matches commands that only preview what they would do.
var dryRun = regexp.MustCompile(`^(brew (cleanup|autoremove)|mo (clean|optimize|uninstall|purge)|topgrade)\b.*(\s--dry-run|\s-n)\b`)

var (
	plainCommand = regexp.MustCompile(`^[\w.+-]+$`)
	subcommandRe = regexp.MustCompile(`^[a-z][\w-]*$`)
)

const (
	// helpTools have subcommands and answer "help", "version" and -h with
	// text and nothing else.
	helpTools = "brew git docker podman npm pnpm yarn bun pip pip3 pipx uv cargo rustup gem go gh mas mo topgrade " +
		"kubectl helm terraform aws gcloud az apt apt-get dnf zypper flatpak snap winget scoop choco omega ollama lms " +
		"mise asdf deno swift tmutil softwareupdate launchctl defaults diskutil"
	// versionTools print their version for -v, -V or -version.
	versionTools = "node npm pnpm yarn bun ruby java javac python python3 perl php lua go rustc cargo gem pip pip3 " +
		"tmux nvim vim gcc clang make cmake swift deno ffmpeg xcodebuild brew git docker curl wget ssh zsh bash terraform kubectl helm"
	// noHelp take --help for a name to act on, or never stop printing it.
	noHelp = "yes rm rmdir rd ri del erase remove-item mkdir md ni new-item touch kill killall pkill shutdown reboot halt poweroff"
)

// helpOnly reports a command that only asks a tool what it is: "--help" or
// "--version" alone after the program, "help", "version", -h or -V alone
// after a tool known to answer them, or "--help" after the subcommands of
// such a tool. "rm foo help" deletes two files; it used to pass.
func helpOnly(p string) bool {
	f := strings.Fields(p)
	if len(f) < 2 || len(f) > 4 || !plainCommand.MatchString(f[0]) {
		return false
	}
	tool, last := strings.ToLower(f[0]), f[len(f)-1]
	if len(f) == 2 {
		switch last {
		case "--help", "--version":
			return !has(noHelp, tool)
		case "help", "version", "-h":
			return has(helpTools, tool)
		case "-v", "-V", "-version":
			return has(versionTools, tool)
		}
		return false
	}
	if !has(helpTools, tool) {
		return false
	}
	mid := f[1 : len(f)-1]
	if f[1] == "help" {
		mid = f[2:] // brew help cleanup
	} else if last != "--help" {
		return false
	}
	for _, w := range mid {
		if !subcommandRe.MatchString(w) {
			return false
		}
	}
	return true
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
	expanded := homeVar.ReplaceAllString(arg, home)
	if strings.HasPrefix(expanded, "~") {
		rest := strings.TrimPrefix(expanded, "~")
		if rest != "" && !strings.HasPrefix(rest, "/") {
			return false
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
	switch p := path.Clean(expanded); p {
	case "/", "/Users", "/System", "/Library", "/Applications",
		"/home", "/root", "/etc", "/usr", "/var", "/boot", "/bin", "/sbin", "/lib", "/lib64", "/opt":
		return true
	default:
		return p == home
	}
}

func classifyOne(part string) Risk {
	if wipesMachine(part) || leaksSecret(part) || windowsWipes(part) {
		return Blocked
	}
	words := shellWords(part)
	// Strip leading env assignments such as LC_ALL=C cmd. One that can
	// change what the command runs makes it a change, whatever follows.
	floor := Safe
	for len(words) > 1 && strings.Contains(words[0].text, "=") && !strings.HasPrefix(words[0].text, "-") {
		if !harmlessVars.MatchString(words[0].text) {
			floor = Dangerous
		}
		words = words[1:]
	}
	r := classifyBare(joinText(words), joinRaw(words))
	// A command is only read-only, or routine, when its text says what
	// the program receives.
	if r < Dangerous && (rewritesWords(words) || !bracesOK(joinRaw(words))) {
		r = Dangerous
	}
	if r > floor {
		return r
	}
	return floor
}

// classifyBare decides one command with its leading assignments removed.
// p holds its words as the program receives them, without quotes; raw is
// the command as it was typed, which is what PowerShell reads.
func classifyBare(p, raw string) Risk {
	if p == "" || p == "true" || p == "false" || p == ":" {
		return Safe
	}
	if strings.HasPrefix(p, "sudo ") || p == "sudo" {
		return Dangerous
	}
	// env runs what follows it: "env rm -rf x" is rm, not a look at the
	// environment. Alone, or with only assignments, it prints variables.
	f := strings.Fields(p)
	if f[0] == "env" {
		i := 1
		floor := Safe
		for i < len(f) && (strings.HasPrefix(f[i], "-") || strings.Contains(f[i], "=")) {
			switch {
			case f[i] == "-u" || f[i] == "--unset":
				i++
			case f[i] == "-i" || f[i] == "-" || f[i] == "--ignore-environment" || f[i] == "-0" || f[i] == "--null" || strings.HasPrefix(f[i], "--unset="):
			case strings.HasPrefix(f[i], "-"):
				// -S runs the string that follows, -C changes directory,
				// -P changes the search path: not a look at the environment.
				return Dangerous
			case !harmlessVars.MatchString(f[i]):
				floor = Dangerous
			}
			i++
		}
		if i >= len(f) {
			return floor
		}
		if r := classifyOne(strings.Join(f[i:], " ")); r > floor {
			return r
		}
		return floor
	}
	if neverEnds(f) || writesByOption(p) {
		return Dangerous
	}
	if risk, ok := queryTool(p); ok {
		return risk
	}
	if risk, ok := classifyWindows(raw); ok {
		return risk
	}
	if dryRun.MatchString(p) || helpOnly(p) {
		return Safe
	}
	if f[0] == "find" && findChanges(f[1:]) {
		return Dangerous
	}
	if sedInPlace.MatchString(p) {
		return Dangerous
	}
	if strings.HasPrefix(p, "journalctl ") && journalWrites.MatchString(p) {
		return Dangerous
	}
	for _, pre := range safePrefixes {
		if hasPrefixWord(p, pre) {
			return Safe
		}
	}
	for _, pre := range normalPrefixes {
		if hasPrefixWord(p, pre) {
			return Normal
		}
	}
	return Dangerous
}

// contentReaders read what they are pointed at to the end.
const contentReaders = "cat head tail wc grep egrep fgrep rg sort uniq jq awk cut tr shasum md5 md5sum sha256sum diff"

// neverEnds reports a read-only command that does not finish by itself:
// it follows a file, repeats at an interval, or reads a device with no
// end. Run unasked it would hold the run until the timeout.
func neverEnds(f []string) bool {
	args := f[1:]
	short := func(letters string) bool {
		for _, a := range args {
			if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.ContainsAny(a, letters) {
				return true
			}
		}
		return false
	}
	long := func(names ...string) bool {
		for _, a := range args {
			name, _, _ := strings.Cut(a, "=")
			for _, n := range names {
				if name == n {
					return true
				}
			}
		}
		return false
	}
	switch f[0] {
	case "tail":
		if short("fF") || long("--follow", "--retry") {
			return true
		}
	case "journalctl":
		if short("f") || long("--follow") {
			return true
		}
	case "vm_stat":
		// "vm_stat 5" repeats for ever; -c gives it a count.
		return subcommand(args) != "" && !short("c")
	case "top":
		for i, a := range args {
			if a == "-l0" || (a == "-l" && (i+1 >= len(args) || args[i+1] == "0")) {
				return true
			}
		}
	case "docker", "podman", "nerdctl":
		switch subcommand(args) {
		case "logs":
			return short("f") || long("--follow")
		case "stats":
			return !long("--no-stream")
		case "events":
			return true
		}
	}
	if has(contentReaders, f[0]) && !(f[0] == "head" && short("c")) {
		for _, a := range args {
			if dev := strings.TrimLeft(a, "<"); strings.HasPrefix(dev, "/dev/") && dev != "/dev/null" {
				return true
			}
		}
	}
	return false
}

func hasPrefixWord(s, prefix string) bool {
	if !strings.HasPrefix(s, prefix) {
		return false
	}
	return len(s) == len(prefix) || s[len(prefix)] == ' ' || strings.HasSuffix(prefix, " ")
}

// splitCommands splits on ;, &&, || and | outside of quotes.
func splitCommands(cmd string) []string { return splitUnquoted(cmd, true, true) }

// splitUnquoted splits a command outside of quotes: at the pipes (|) when
// pipes is set, and at what separates one pipeline from the next (;, a new
// line, &&, || and &) when lists is.
func splitUnquoted(cmd string, pipes, lists bool) []string {
	var parts []string
	var cur strings.Builder
	var quote byte
	cut := func(yes bool, sep string) {
		if yes {
			parts = append(parts, cur.String())
			cur.Reset()
		} else {
			cur.WriteString(sep)
		}
	}
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
			cur.WriteByte(c)
		case c == '\'' || c == '"':
			quote = c
			cur.WriteByte(c)
		case c == ';' || c == '\n':
			cut(lists, string(c))
		case c == '&' && i+1 < len(cmd) && cmd[i+1] == '&', c == '|' && i+1 < len(cmd) && cmd[i+1] == '|':
			cut(lists, cmd[i:i+2])
			i++
		case c == '|':
			cut(pipes, "|")
		case c == '&' && ((i > 0 && cmd[i-1] == '>') || (i+1 < len(cmd) && cmd[i+1] == '>')):
			// Part of a redirection such as 2>&1 or &>.
			cur.WriteByte(c)
		case c == '&':
			// Backgrounding; treat as separator.
			cut(lists, "&")
		default:
			cur.WriteByte(c)
		}
	}
	parts = append(parts, cur.String())
	return parts
}
