package safety

import (
	"path"
	"strings"
)

// Deleting or overwriting the user's own files is asked about every time,
// whatever the approval mode: what goes was not downloaded and cannot be
// downloaded again. A path counts when it is written from the home folder
// (outside the caches regenerable lists), and also when it is relative and
// the shell is in the home folder after a cd, or in a folder nobody can
// tell from the command: "rm -rf node_modules" deletes wherever it runs.

// askDir is where "cd args" leaves a shell that was in dir, "" when that
// cannot be known from the command. Without an argument cd goes home.
func askDir(dir string, args []string) string {
	to := "~"
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			to = a
			break
		}
		if a == "-" {
			return "" // the previous folder
		}
	}
	if !relativePath(to) {
		return to
	}
	if dir == "" {
		return ""
	}
	return dir + "/" + to
}

// relativePath reports a word that names a path under the working directory.
func relativePath(w string) bool {
	return w != "" && !strings.HasPrefix(w, "/") && !strings.HasPrefix(w, "~") && !strings.HasPrefix(w, "$") &&
		!strings.HasPrefix(w, "-") && !driveLetter.MatchString(w)
}

// homeTarget reports a path in the user's files: one written from the home
// folder, or a relative one when the shell is in the home folder or in a
// folder the command does not say.
func homeTarget(a, dir string) bool {
	if a == "" || strings.HasPrefix(a, "-") {
		return false
	}
	if underHome(a) {
		return true
	}
	if !relativePath(a) {
		return false
	}
	if dir == "" {
		return true
	}
	return underHome(dir + "/" + a)
}

// deletesInHome reports a command that deletes or overwrites files of the
// user: rm and unlink (recursive or not), find with -delete or -exec rm,
// rsync --delete into a folder of the user, shred and truncate, and rm
// handed its names by xargs, which the command does not show.
func deletesInHome(seg []string, verb string, args []string, dir string) bool {
	switch verb {
	case "rm", "unlink", "srm":
		// The names come from standard input: nobody can tell what they are.
		for _, w := range seg[:len(seg)-len(args)-1] {
			if path.Base(w) == "xargs" || path.Base(w) == "parallel" {
				return true
			}
		}
		for _, a := range rejoin(operands(args, "")) {
			if homeTarget(a, dir) {
				return true
			}
		}
	case "find":
		if !findDeletes(args) {
			return false
		}
		roots := findRoots(args)
		if len(roots) == 0 {
			roots = []string{"."}
		}
		for _, r := range roots {
			// What goes is what is inside: ~/.cache/thumbnails is a cache.
			if homeTarget(strings.TrimSuffix(r, "/")+"/x", dir) {
				return true
			}
		}
	case "rsync":
		return rsyncDeletesHome(args, dir)
	case "shred":
		for _, a := range rejoin(operands(args, "-n --iterations -s --size --random-source")) {
			if homeTarget(a, dir) {
				return true
			}
		}
	case "truncate":
		for _, a := range rejoin(operands(args, "-s --size -r --reference")) {
			if underHome(a) {
				return true
			}
		}
	case "tee":
		if anyArg(args, func(a string) bool {
			return abbrev(a, "--append") || (shortFlags.MatchString(a) && strings.Contains(a, "a"))
		}) {
			return false
		}
		for _, a := range rejoin(operands(args, "--output-error")) {
			if underHome(a) {
				return true
			}
		}
	}
	return false
}

// operands are the arguments that are not options, nor the value of one of
// the options named in takeValue. Everything after -- is an operand.
func operands(args []string, takeValue string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return append(out, args[i+1:]...)
		case has(takeValue, a):
			i++
		case strings.HasPrefix(a, "-") && a != "-":
		default:
			out = append(out, a)
		}
	}
	return out
}

// findDeletes reports a find that deletes what it finds.
func findDeletes(args []string) bool {
	for i, a := range args {
		switch a {
		case "-delete":
			return true
		case "-exec", "-execdir", "-ok", "-okdir":
			if i+1 < len(args) && has("rm unlink shred srm", path.Base(args[i+1])) {
				return true
			}
		}
	}
	return false
}

// findRoots are the folders find starts from: the words before the first
// expression.
func findRoots(args []string) []string {
	var roots []string
	for _, a := range args {
		if a == "-H" || a == "-L" || a == "-P" || a == "-E" || a == "-x" || a == "-X" || a == "-s" || a == "-d" {
			continue // BSD and GNU options before the paths
		}
		if strings.HasPrefix(a, "-") || a == "(" || a == "!" || a == `\(` {
			break
		}
		roots = append(roots, a)
	}
	return roots
}

// rsyncDeletesHome reports rsync told to delete in a folder of the user:
// --delete and its variants empty the destination of what the source
// lacks, and --remove-source-files deletes the sources once copied.
func rsyncDeletesHome(args []string, dir string) bool {
	deletes, removesSources := false, false
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "--del"):
			deletes = true
		case a == "--remove-source-files":
			removesSources = true
		}
	}
	paths := operands(args, "-e --rsh --rsync-path --exclude --include --filter -f --exclude-from --include-from --files-from --password-file --log-file --temp-dir -T --compare-dest --copy-dest --link-dest --partial-dir --backup-dir --suffix --chmod --chown --usermap --groupmap --timeout --contimeout --port --bwlimit --max-size --min-size --max-delete --block-size -B --info --debug")
	if len(paths) == 0 {
		return false
	}
	local := func(p string) bool { return !strings.Contains(p, ":") }
	if deletes {
		if dst := paths[len(paths)-1]; local(dst) && homeTarget(strings.TrimSuffix(dst, "/")+"/x", dir) {
			return true
		}
	}
	if removesSources {
		for _, src := range paths[:len(paths)-1] {
			if local(src) && homeTarget(src, dir) {
				return true
			}
		}
	}
	return false
}

// overwritesInHome reports an output redirection that empties a file of
// the user: ": > ~/Documents/thesis.tex". Appending (>>) and duplicating a
// descriptor (2>&1) lose nothing.
func overwritesInHome(command string) bool {
	cmd := stripComments(command)
	var quote byte
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
			continue
		case c == '\\':
			i++
			continue
		case c == '\'' || c == '"':
			quote = c
			continue
		case c != '>':
			continue
		}
		if i+1 < len(cmd) && cmd[i+1] == '>' {
			i++ // >> appends
			continue
		}
		if i+1 < len(cmd) && cmd[i+1] == '&' {
			continue // >&2
		}
		if i > 0 && cmd[i-1] == '<' {
			continue // <> opens for reading too and truncates nothing
		}
		j := i + 1
		if j < len(cmd) && cmd[j] == '|' {
			j++ // >| overrides noclobber
		}
		for j < len(cmd) && (cmd[j] == ' ' || cmd[j] == '\t') {
			j++
		}
		k := j
		for k < len(cmd) && strings.IndexByte(" \t\n;|&<>()", cmd[k]) < 0 {
			k++
		}
		if target := dequote(cmd[j:k]); underHome(target) {
			return true
		}
		i = k - 1
	}
	return false
}
