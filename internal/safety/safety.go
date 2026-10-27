// Package safety classifies shell commands by risk so the agent knows which
// ones it may run on its own, which need the user's approval, and which it
// must never run.
package safety

import (
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
	regexp.MustCompile(`\brm\s+(-\S*\s+)*(/|~|\$HOME|/\*|~/\*|/Users|/System|/Library|/Applications)/?\s*($|[;&|])`),
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
	"docker system df", "docker ps", "docker images", "git status", "git log", "git --version",
	"topgrade --dry-run", "topgrade -n", "topgrade --version", "topgrade --config-reference",
	"mo --version", "mo clean --dry-run", "mo optimize --dry-run", "mo uninstall --dry-run", "mo history",
	"mo --help", "mo help",
}

// Routine, recoverable update operations.
var normalPrefixes = []string{
	"brew update", "brew upgrade", "brew fetch", "mas upgrade",
	"npm update -g", "npm update --global", "pnpm update -g", "pnpm up -g", "bun upgrade",
	"pipx upgrade", "pipx upgrade-all", "uv tool upgrade", "rustup update", "cargo install-update",
	"gh extension upgrade", "mo update", "gem update --user-install", "tldr --update",
}

var (
	findDestructive = regexp.MustCompile(`\s-(delete|exec|execdir|ok|okdir)\b`)
	sedInPlace      = regexp.MustCompile(`\bsed\b.*\s-i`)
	fdDup           = regexp.MustCompile(`[0-9]*>&[0-9-]+`)
	devNullRedirect = regexp.MustCompile(`[0-9&]?>{1,2}\s*/dev/null`)
)

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
	if strings.Contains(cmd, "$(") || strings.Contains(cmd, "`") || strings.Contains(cmd, "<(") {
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

func classifyOne(part string) Risk {
	p := strings.TrimSpace(part)
	// Strip leading env assignments such as FOO=1 cmd.
	for {
		f := strings.Fields(p)
		if len(f) > 1 && strings.Contains(f[0], "=") && !strings.HasPrefix(f[0], "-") {
			p = strings.TrimSpace(strings.TrimPrefix(p, f[0]))
			continue
		}
		break
	}
	if p == "" || p == "true" || p == "false" {
		return Safe
	}
	if strings.HasPrefix(p, "sudo ") || p == "sudo" {
		return Dangerous
	}
	if strings.HasPrefix(p, "find ") && findDestructive.MatchString(p) {
		return Dangerous
	}
	if sedInPlace.MatchString(p) {
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
