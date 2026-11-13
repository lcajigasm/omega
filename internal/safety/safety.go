// Package safety classifies shell commands by risk so the agent knows which
// ones it may run on its own, which need the user's approval, and which it
// must never run.
package safety

import (
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
	regexp.MustCompile(`\bmkfs\b`),
	regexp.MustCompile(`\bdiskutil\s+(erase\w*|partition\w*|zero\w*|secureErase|apfs\s+delete\w*)`),
	regexp.MustCompile(`\bdd\b.*\bof=/dev/`),
	regexp.MustCompile(`>\s*/dev/(r?disk|sd)`),
	regexp.MustCompile(`\bcsrutil\s+(disable|clear|authenticated-root)`),
	regexp.MustCompile(`\bnvram\b`),
	regexp.MustCompile(`\bspctl\s+--master-disable`),
	regexp.MustCompile(`:\(\)\s*\{`),
	regexp.MustCompile(`\b(curl|wget)\b[^|]*\|\s*(sudo\s+)?(ba|z|)sh\b`),
	regexp.MustCompile(`\bchmod\s+(-R\s+)?[0-7]*777\s+/(\s|$)`),
	regexp.MustCompile(`\bfdesetup\b`),
	regexp.MustCompile(`\btmutil\s+(delete|disable)`),
	// Linux: wiping whole disks and switching off rm's own safety net.
	regexp.MustCompile(`\b(wipefs|blkdiscard)\b`),
	regexp.MustCompile(`>\s*/dev/(nvme|vd|mmcblk|hd)`),
	regexp.MustCompile(`--no-preserve-root`),
	// The privacy database is only ever changed through tccutil: a direct
	// write can grant a permission the user never approved.
	// The helper that answers sudo with the password omega holds for this
	// run. Only sudo calls it: a command that names it wants the password.
	regexp.MustCompile(`(?i)sudo_askpass|askpass\.sock|omega_askpass_sock`),
	regexp.MustCompile(`(?i)TCC\.db.*\b(insert|update|delete|replace|drop|alter)\b|\b(insert|update|delete|replace|drop|alter)\b.*TCC\.db`),
}

// Read-only command prefixes.
var safePrefixes = []string{
	"ls", "cat", "head", "tail", "wc", "grep", "rg", "file", "stat", "which", "type", "command -v",
	"echo", "printf", "pwd", "whoami", "id", "date", "uptime", "uname", "sw_vers", "hostname",
	"df", "du", "vm_stat", "sysctl", "ps", "top -l", "pmset -g", "ioreg", "system_profiler",
	"defaults read", "launchctl list", "launchctl print", "softwareupdate -l", "softwareupdate --list",
	"xcode-select -p", "xcode-select --print-path", "diskutil list", "diskutil info", "diskutil apfs list",
	"tmutil listlocalsnapshots", "csrutil status", "mdutil -s", "find", "sort", "uniq", "jq",
	"brew list", "brew info", "brew outdated", "brew doctor", "brew config", "brew --version", "brew --prefix",
	"brew deps", "brew leaves", "brew tap", "brew search", "brew services list", "brew cleanup -n", "brew cleanup --dry-run",
	"brew autoremove -n", "brew autoremove --dry-run",
	"mas list", "mas outdated", "npm ls", "npm list", "npm outdated", "npm config get", "npm doctor",
	"pnpm ls", "pnpm outdated", "pip list", "pip3 list", "pip show", "pip3 show", "pip cache info", "pip3 cache info",
	"pipx list", "uv tool list", "uv cache dir", "cargo install --list", "rustup show", "rustup check",
	"gem list", "gh extension list", "go version", "go env", "node --version", "python3 --version",
	"docker system df", "docker ps", "docker images", "docker image ls", "docker container ls", "docker volume ls",
	"docker info", "docker version", "docker builder du", "docker buildx du",
	"git status", "git log", "git --version", "git worktree list", "git branch", "git remote -v", "git rev-parse",
	"git stash list", "ollama list", "ollama ps", "lms ls", "lms ps", "tmutil listbackups", "mdfind", "sw_vers",
	"pmset -g batt", "brew services info", "npm config list", "pip3 config list",
	"env", "printenv", "git config --list", "git config --global --list", "git config --get", "git config --global --get",
	"git config -l", "pnpm store path", "pnpm config list", "npm config get", "pipx environment", "pipx --version",
	"uv cache dir", "uv python list", "uv --version", "rustup toolchain list", "go env", "brew --cache", "brew --repo",
	"time zsh -i -c exit", "docker context ls", "launchctl list", "tmutil destinationinfo", "mdutil -s",
	"topgrade --dry-run", "topgrade -n", "topgrade --version", "topgrade --config-reference",
	"mo --version", "mo clean --dry-run", "mo optimize --dry-run", "mo uninstall --dry-run", "mo history",
	"mo --help", "mo help",
	// Linux.
	"test", "journalctl", "coredumpctl list", "coredumpctl info", "zypper --non-interactive list-updates",
	"free", "lsb_release", "lscpu", "lsblk", "findmnt", "nproc",
	// systemctl, systemd-analyze, loginctl, hostnamectl, rpm, dpkg, pacman,
	// paccache, apt-get and dnf history are decided by queryTool: each also
	// has subcommands and flags that change the system.
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
	findDestructive = regexp.MustCompile(`\s-(delete|exec|execdir|ok|okdir|fprint|fprint0|fprintf|fls)\b`)
	sedInPlace      = regexp.MustCompile(`\bsed\b.*\s-i`)
	// journalctl only reads, except when it trims, rotates or rewrites the journal.
	journalWrites   = regexp.MustCompile(`\s--(vacuum-\w+|rotate|flush|sync|relinquish-var|smart-relinquish-var|setup-keys|update-catalog)\b`)
	fdDup           = regexp.MustCompile(`[0-9]*>&[0-9-]+`)
	devNullRedirect = regexp.MustCompile(`[0-9&]?>{1,2}\s*/dev/null`)
)

var dockerDestructive = regexp.MustCompile(`\bdocker\s+((system|image|container|volume|network|builder|buildx)\s+prune|rmi|rm|volume\s+rm|image\s+rm|container\s+rm|buildx\s+rm)\b`)

// AlwaysConfirm reports commands that must be confirmed by the user every
// time, even under --yes: Docker cleanups can delete images, volumes and
// databases that are expensive or impossible to recreate.
func AlwaysConfirm(command string) bool {
	return dockerDestructive.MatchString(command)
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
	// Secrets and wipes are looked for in the whole command, through
	// wrappers, quoting and command substitution, before anything else.
	if leaksSecret(cmd) {
		return Blocked
	}
	for _, seg := range shellSegments(cmd) {
		if run := afterWrappers(seg); len(run) > 0 && wipesMachine(strings.Join(run, " ")) {
			return Blocked
		}
	}
	if strings.Contains(cmd, "$(") || strings.Contains(cmd, "`") || strings.Contains(cmd, "<(") {
		return Dangerous
	}
	if hidesCommands(cmd) {
		return Dangerous
	}
	// Any output redirection other than fd duplication or /dev/null writes a file.
	if strings.Contains(fdDup.ReplaceAllString(devNullRedirect.ReplaceAllString(cmd, ""), ""), ">") {
		return Dangerous
	}
	max := Safe
	for _, part := range splitCommands(cmd) {
		r := classifyOne(part)
		if r > max {
			max = r
		}
	}
	return max
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
//   - any parenthesis outside quotes: a subshell, zsh's =(cmd), the glob
//     qualifier *(e:'cmd':), or a PowerShell expression (Remove-Item x).
func hidesCommands(cmd string) bool {
	if escapedBreak.MatchString(cmd) || strings.Contains(cmd, "$'") {
		return true
	}
	return strings.Contains(unquoted(cmd), "(")
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
// writes a file, changes a setting or runs another program.
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
	positional := func() (n int) {
		for _, a := range args {
			if !strings.HasPrefix(a, "-") {
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
		return positional() > 1
	case "date":
		return short("s") || opt("--set") || positional() > 0 && !strings.HasPrefix(firstPositional(args), "+")
	case "hostname":
		return positional() > 0 || short("F") || opt("--file")
	case "sysctl":
		return short("w") || short("p") || strings.Contains(strings.Join(args, " "), "=")
	case "go":
		return len(args) > 0 && args[0] == "env" && (opt("-w", "-u"))
	case "brew":
		return len(args) > 0 && args[0] == "tap" && positional() > 1
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

func firstPositional(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}

// dryRun matches commands that only preview what they would do.
var dryRun = regexp.MustCompile(`^(brew (cleanup|autoremove)|mo (clean|optimize|uninstall|purge)|topgrade)\b.*(\s--dry-run|\s-n)\b`)

// helpOnly matches "<tool> --help" or "<tool> --version" with nothing else.
var helpOnly = regexp.MustCompile(`^[\w.+-]+( [a-z][\w-]*)? (--help|-h|--version|-V|version|help)$`)

// homeSentinel stands in for ~ and $HOME so path.Clean can see that
// "$HOME/.." is /Users and "~" itself is the home directory.
const homeSentinel = "/Users/omega"

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
	destructive := false
	root := false
	for _, a := range args {
		switch a {
		case "-delete", "-exec", "-execdir", "-ok", "-okdir":
			destructive = true
		}
		if isRootTarget(a) {
			root = true
		}
	}
	return destructive && root
}

func isRootTarget(arg string) bool {
	if arg == "" || strings.HasPrefix(arg, "-") {
		return false
	}
	expanded := strings.ReplaceAll(arg, "${HOME}", homeSentinel)
	expanded = strings.ReplaceAll(expanded, "$HOME", homeSentinel)
	if strings.HasPrefix(expanded, "~") {
		rest := strings.TrimPrefix(expanded, "~")
		if rest != "" && !strings.HasPrefix(rest, "/") {
			return false
		}
		expanded = homeSentinel + rest
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
	switch path.Clean(expanded) {
	case "/", homeSentinel, "/Users", "/System", "/Library", "/Applications",
		"/home", "/root", "/etc", "/usr", "/var", "/boot", "/bin", "/sbin", "/lib", "/lib64", "/opt":
		return true
	}
	return false
}

func classifyOne(part string) Risk {
	if wipesMachine(part) || leaksSecret(part) || windowsWipes(part) {
		return Blocked
	}
	p := strings.TrimSpace(part)
	// Strip leading env assignments such as LC_ALL=C cmd. One that can
	// change what the command runs makes it a change, whatever follows.
	floor := Safe
	for {
		f := strings.Fields(p)
		if len(f) > 1 && strings.Contains(f[0], "=") && !strings.HasPrefix(f[0], "-") {
			if !harmlessVars.MatchString(f[0]) {
				floor = Dangerous
			}
			p = strings.TrimSpace(strings.TrimPrefix(p, f[0]))
			continue
		}
		break
	}
	if r := classifyBare(p); r > floor {
		return r
	}
	return floor
}

// classifyBare decides one command with its leading assignments removed.
func classifyBare(p string) Risk {
	if p == "" || p == "true" || p == "false" {
		return Safe
	}
	if strings.HasPrefix(p, "sudo ") || p == "sudo" {
		return Dangerous
	}
	// env runs what follows it: "env rm -rf x" is rm, not a look at the
	// environment. Alone, or with only assignments, it prints variables.
	if f := strings.Fields(p); f[0] == "env" {
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
	if writesByOption(p) {
		return Dangerous
	}
	if risk, ok := queryTool(p); ok {
		return risk
	}
	if risk, ok := classifyWindows(p); ok {
		return risk
	}
	if dryRun.MatchString(p) || helpOnly.MatchString(p) {
		return Safe
	}
	if strings.HasPrefix(p, "find ") && findDestructive.MatchString(p) {
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

func hasPrefixWord(s, prefix string) bool {
	if !strings.HasPrefix(s, prefix) {
		return false
	}
	return len(s) == len(prefix) || s[len(prefix)] == ' ' || strings.HasSuffix(prefix, " ")
}

// splitCommands splits on ;, &&, || and | outside of quotes.
func splitCommands(cmd string) []string {
	var parts []string
	var cur strings.Builder
	var quote byte
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
			parts = append(parts, cur.String())
			cur.Reset()
		case c == '&' && i+1 < len(cmd) && cmd[i+1] == '&', c == '|' && i+1 < len(cmd) && cmd[i+1] == '|':
			parts = append(parts, cur.String())
			cur.Reset()
			i++
		case c == '|':
			parts = append(parts, cur.String())
			cur.Reset()
		case c == '&' && ((i > 0 && cmd[i-1] == '>') || (i+1 < len(cmd) && cmd[i+1] == '>')):
			// Part of a redirection such as 2>&1 or &>.
			cur.WriteByte(c)
		case c == '&':
			// Backgrounding; treat as separator.
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	parts = append(parts, cur.String())
	return parts
}
