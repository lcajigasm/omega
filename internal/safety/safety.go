// Package safety classifies shell commands by risk so the agent knows which
// ones it may run on its own, which need the user's approval, and which it
// must never run.
package safety

import (
	"path"
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
	regexp.MustCompile(`\b(curl|wget)\b[^|]*\|\s*(sudo\s+(-\S+\s+)*)?(env\s+)?(\S*/)?(ba|z|da|k|fi|)sh\b`),
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
	// Wiping a disk with the tools blockedTools does not name: a GPT
	// zapped or cleared, a device shredded, a volume erased by a restore.
	regexp.MustCompile(`\bsgdisk\b[^;|&]*\s(--zap-all|--zap|--clear|-[A-Za-z]*[Zzo][A-Za-z]*)(\s|$)`),
	regexp.MustCompile(`\bshred\b[^;|&]*\s/dev/`),
	regexp.MustCompile(`\basr\b[^;|&]*\s-{1,2}erase\b`),
	// What was last copied is often a password.
	regexp.MustCompile(`(?i)\b(get-clipboard|gcb|pbpaste|wl-paste)\b|\bxclip\b.*\s-o\b|\bxsel\b.*\s(-o|--output)\b`),
}

const wholeDisk = `(r?disk[0-9]|sd[a-z]|nvme[0-9]|vd[a-z]|mmcblk[0-9]|hd[a-z])`

// blockedTools format disks, erase firmware variables or turn disk
// encryption off. They are matched by name wherever one can run; the name
// alone is not the tool, so "man mkfs" and "ls /usr/sbin | grep nvram" pass.
var blockedTools = regexp.MustCompile(`\b(mkfs|mke2fs|mkswap|newfs(_\w+)?|wipefs|blkdiscard|nvram|fdesetup)\b`)

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

// Classify returns the highest risk among the parts of a (possibly compound) command.
func Classify(command string) Risk {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return Safe
	}
	// The never-list is read twice: as typed, and as the shell hands it to
	// the program, without quotes or backslashes (dd of="/dev/disk0",
	// csrutil 'disable', curl … | /bin/sh).
	plain := dequote(cmd)
	for _, re := range blocked {
		if re.MatchString(cmd) || re.MatchString(plain) {
			return Blocked
		}
	}
	if runsBlockedTool(cmd) {
		return Blocked
	}
	if namesKeptSecret(plain) || namesSecretFileVar(plain) {
		return Blocked
	}
	// Secrets and wipes are looked for in the whole command, through
	// wrappers, quoting and command substitution, before anything else.
	if leaksSecret(cmd) || namesKeptSecret(cmd) {
		return Blocked
	}
	if wipesAnywhere(cmd) || windowsPipeWipes(cmd) || stopsMachine(cmd) {
		return Blocked
	}
	for _, part := range splitCommands(cmd) {
		if windowsWipes(part) {
			return Blocked
		}
	}
	if sudoEnvChanged(cmd) {
		return Blocked
	}
	if hidesCommands(cmd) || uncPath.MatchString(cmd) {
		return Dangerous
	}
	// From here the command is read as the shell reads it: without comments.
	code := arithmetic.ReplaceAllString(stripComments(cmd), "0")
	if strings.Contains(code, "`") {
		return Dangerous
	}
	// A $(…) is read-only when what runs inside is, and its output cannot
	// turn into an option (see substitutions).
	code, inners, ok := substitutions(code)
	if !ok {
		return Dangerous
	}
	// Any other parenthesis outside quotes: a subshell, <(cmd), zsh's
	// =(cmd), the glob qualifier *(e:'cmd':), or a PowerShell expression
	// (Remove-Item x). find's \( … \) is only a word.
	bare := unquoted(code)
	if noGroups, ok := withoutFindGroups(bare); !ok || strings.Contains(noGroups, "(") {
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
	if max == Safe && (envDumpReshaped(code) || startupFileReshaped(code) || loopVarsMisused(code, inners)) {
		return Dangerous
	}
	return max
}

// dequote is a command without its quotes, backticks and backslashes, and
// with repeated slashes as one: close to the words the programs receive.
func dequote(cmd string) string {
	s := strings.NewReplacer("'", "", `"`, "", "`", "", `\`, "").Replace(cmd)
	for strings.Contains(s, "//") && !strings.Contains(s, "://") {
		s = strings.ReplaceAll(s, "//", "/")
	}
	return s
}

// abbrev reports a long option written in full or shortened: getopt_long
// takes any unambiguous start, so --out is --output for sort.
func abbrev(arg, full string) bool {
	name, _, _ := strings.Cut(arg, "=")
	return name == full || (len(name) > 2 && strings.HasPrefix(name, "--") && strings.HasPrefix(full, name))
}
