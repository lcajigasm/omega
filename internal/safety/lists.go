package safety

import (
	"regexp"
	"strings"
)

// The prefix lists: what is read-only or a routine update by its first
// words, and the commands that only ask a tool for help or its version.

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
	"git stash list", "git rev-list", "git ls-files", "git ls-tree", "git describe", "git merge-base", "git show-ref",
	"git for-each-ref", "git count-objects", "git shortlog", "git blame", "git cat-file", "git name-rev", "git reflog show",
	"time -p zsh -i -c exit", "ollama list", "ollama ps", "lms ls", "lms ps", "tmutil listbackups", "mdfind", "sw_vers",
	"pmset -g batt", "brew services info", "npm config list", "pip3 config list",
	"env", "printenv", "git config --list", "git config --global --list", "git config --get", "git config --global --get",
	"git config -l", "pnpm store path", "pnpm config list", "npm config get", "pipx environment", "pipx --version",
	"uv cache dir", "uv python list", "uv --version", "rustup toolchain list", "go env", "brew --cache", "brew --repo",
	"time zsh -i -c exit", "docker context ls", "launchctl list", "tmutil destinationinfo",
	"mise ls", "mise list", "mise current", "mise outdated", "mise doctor", "mise config ls", "mise --version",
	"ssh -G",
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

func hasPrefixWord(s, prefix string) bool {
	if !strings.HasPrefix(s, prefix) {
		return false
	}
	return len(s) == len(prefix) || s[len(prefix)] == ' ' || strings.HasSuffix(prefix, " ")
}
