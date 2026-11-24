package safety

import (
	"regexp"
	"strings"
)

// Classifying one simple command, after Classify has split the line.

func classifyOne(part string) Risk {
	if wipesMachine(part) || leaksSecret(part) || windowsWipes(part) {
		return Blocked
	}
	// "do cmd", "then cmd", "if cmd": the keyword only says where cmd runs.
	words := dropKeywords(shellWords(part))
	// Strip leading env assignments such as LC_ALL=C cmd. One that can
	// change what the command runs makes it a change, whatever follows.
	floor := Safe
	for len(words) > 1 && strings.Contains(words[0].text, "=") && !strings.HasPrefix(words[0].text, "-") {
		floor = max(floor, assignmentRisk(words[0].text))
		words = words[1:]
	}
	r, compound := classifyCompound(words)
	if !compound {
		r = classifyBare(joinText(words), joinRaw(words))
	}
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
			default:
				floor = max(floor, assignmentRisk(f[i]))
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
	// /usr/bin/log is log: the system folders are protected by SIP and
	// nobody but the system puts a program there.
	if sysPath.MatchString(f[0]) {
		base := f[0][strings.LastIndex(f[0], "/")+1:]
		return classifyBare(base+p[len(f[0]):], base+strings.TrimPrefix(raw, f[0]))
	}
	switch f[0] {
	case "xargs":
		return classifyXargs(f[1:])
	case "git":
		// git -C repo status is git status: the options that only say
		// where to look are dropped. -c and --git-dir stay a change.
		if rest, ok := gitWhere(f[1:]); ok {
			return classifyBare("git "+strings.Join(rest, " "), "git "+strings.Join(rest, " "))
		}
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

// sysPath is a program named by its place in the system folders.
var sysPath = regexp.MustCompile(`^/(usr/)?s?bin/[\w.-]+$`)

// gitWhere drops git's options that only choose the repository (-C dir)
// or the pager. ok is false when there were none.
func gitWhere(args []string) ([]string, bool) {
	i := 0
	for i < len(args) {
		switch a := args[i]; {
		case a == "-C" && i+1 < len(args):
			i += 2
		case has("--no-pager -P --no-optional-locks --literal-pathspecs --no-replace-objects", a):
			i++
		default:
			return args[i:], i > 0
		}
	}
	return nil, false
}

// xargsRuns are the programs xargs may hand what it reads to and stay
// read-only: they print sizes, names and types, never contents.
const xargsRuns = "du ls stat file wc basename dirname echo"

// classifyXargs decides xargs by the program it runs.
func classifyXargs(args []string) Risk {
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		switch a := args[i]; {
		case has("-0 --null -r --no-run-if-empty -t --verbose", a):
		case has("-n -P -L -s", a):
			i++
		case shortFlags.MatchString(a) && strings.Trim(a, "-0rt") == "":
		default:
			return Dangerous // -I and -J rewrite the command, -o and -p reach the terminal, -a reads a file
		}
		i++
	}
	if i >= len(args) {
		return Safe // echo
	}
	if !has(xargsRuns, args[i]) {
		return Dangerous
	}
	return classifyBare(strings.Join(args[i:], " "), strings.Join(args[i:], " "))
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
	// abbreviated: getopt_long takes any unambiguous start (tail --foll).
	abbreviated := func(names ...string) bool {
		for _, a := range args {
			for _, n := range names {
				if abbrev(a, n) {
					return true
				}
			}
		}
		return false
	}
	switch f[0] {
	case "tail":
		if short("fF") || abbreviated("--follow", "--retry") {
			return true
		}
	case "journalctl":
		if short("f") || abbreviated("--follow") {
			return true
		}
	case "mdfind":
		// -live keeps the query open and prints every change.
		return long("-live")
	case "pmset":
		// The -g logs that stream: pslog, rawlog, thermlog, assertionslog.
		for _, a := range args {
			if has("pslog rawlog thermlog assertionslog", a) {
				return true
			}
		}
	case "log":
		return subcommand(args) == "stream"
	case "fs_usage", "iotop", "dtruss", "opensnoop", "tcpdump", "nettop", "powermetrics":
		return true
	case "iostat":
		// An interval repeats until -c counts the reports.
		return positional(args) > 0 && !short("c")
	case "ping", "ping6":
		return !short("c")
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
			return short("f") || abbreviated("--follow")
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
			for _, n := range names {
				if abbrev(a, n) {
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
	case "jq":
		// env and $ENV are the whole environment, to cut up as it likes.
		return jqEnv.MatchString(p)
	case "pip", "pip3":
		return opt("--log", "--python", "--cache-dir")
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
		if opt("--output", "--ext-diff", "--textconv", "--filters") {
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

var jqEnv = regexp.MustCompile(`\benv\b|\$ENV\b|\$__loc__|input_filename`)

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
		case abbrev(a, "--set"):
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
