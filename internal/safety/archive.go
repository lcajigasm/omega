package safety

import (
	"path"
	"strings"
)

// archivesSecrets reports an archiver packing a folder that holds secrets:
// "tar -cf - ~/.ssh | base64" prints the private keys as surely as cat,
// in a form no mask recognises. tar, zip, 7z, cpio, pax, ditto -c and
// rsync count as readers of every file under the folders they are given.
func archivesSecrets(segs [][]string) bool {
	dir := "" // where an earlier cd left the shell
	for _, seg := range segs {
		run := afterWrappers(seg)
		if len(run) == 0 {
			continue
		}
		verb := strings.ToLower(path.Base(run[0]))
		if verb == "cd" || verb == "pushd" {
			dir = cdTarget(dir, run[1:])
			continue
		}
		roots, ok := archiveSources(verb, run[1:])
		if !ok {
			continue
		}
		for _, r := range roots {
			if r.dir != "" && relativeWord(r.path) {
				r.path = r.dir + "/" + r.path
			} else if dir != "" && relativeWord(r.path) {
				r.path = dir + "/" + r.path
			}
			if unsafeRoot(r.path) || secretFile(r.path, true) {
				return true
			}
		}
	}
	return false
}

// archiveRoot is a path an archiver reads, and the folder it is relative
// to when the archiver was told one (tar -C).
type archiveRoot struct{ path, dir string }

// archiveSources returns what an archiver reads when it creates an
// archive; ok is false for any other program, or an archiver that only
// lists or extracts.
func archiveSources(verb string, args []string) (roots []archiveRoot, ok bool) {
	switch verb {
	case "tar", "bsdtar", "gtar":
		creates, dir := false, ""
		for i := 0; i < len(args); i++ {
			a := args[i]
			switch {
			case a == "-C" || a == "--directory":
				if i+1 < len(args) {
					dir = args[i+1]
					i++
				}
			case strings.HasPrefix(a, "--directory="):
				dir = strings.TrimPrefix(a, "--directory=")
			case a == "-f" || a == "--file" || a == "-T" || a == "--files-from" || a == "-X" || a == "--exclude-from" || a == "-b" || a == "--exclude":
				i++
			case has("--create --append --update --concatenate", a):
				creates = true
			case strings.HasPrefix(a, "--"):
			case strings.HasPrefix(a, "-") || i == 0:
				// A cluster of letters, with or without its dash (tar czf x).
				letters := strings.TrimPrefix(a, "-")
				if strings.ContainsAny(letters, "cru") {
					creates = true
				}
				if strings.Contains(letters, "f") && i+1 < len(args) {
					i++ // its file
				}
			default:
				roots = append(roots, archiveRoot{a, dir})
			}
		}
		return roots, creates
	case "zip":
		pos := operands(args, "-b -n -t -tt -x -i -P --password -O --output-file")
		if len(pos) < 2 {
			return nil, true
		}
		for _, p := range pos[1:] { // the first is the archive
			roots = append(roots, archiveRoot{p, ""})
		}
		return roots, true
	case "7z", "7za", "7zz", "7zr":
		pos := operands(args, "")
		if len(pos) < 3 || pos[0] != "a" && pos[0] != "u" {
			return nil, false
		}
		for _, p := range pos[2:] {
			roots = append(roots, archiveRoot{p, ""})
		}
		return roots, true
	case "pax":
		if !anyArg(args, func(a string) bool { return shortFlags.MatchString(a) && strings.Contains(a, "w") }) {
			return nil, false
		}
		for _, p := range operands(args, "-f -s -x -b -o -B -G -T -U") {
			roots = append(roots, archiveRoot{p, ""})
		}
		return roots, true
	case "ditto":
		pos := operands(args, "--arch --bom")
		if !anyArg(args, func(a string) bool { return a == "-c" || (shortFlags.MatchString(a) && strings.Contains(a, "c")) }) || len(pos) < 2 {
			return nil, false
		}
		for _, p := range pos[:len(pos)-1] {
			roots = append(roots, archiveRoot{p, ""})
		}
		return roots, true
	case "rsync":
		pos := operands(args, "-e --rsh --rsync-path --exclude --include --filter -f --exclude-from --include-from --files-from --password-file --log-file -T --temp-dir")
		if len(pos) < 2 {
			return nil, false
		}
		for _, p := range pos[:len(pos)-1] {
			roots = append(roots, archiveRoot{p, ""})
		}
		return roots, true
	}
	return nil, false
}
