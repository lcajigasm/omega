package agent

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// keepBackups is how many .omega-bak copies of a file write_file keeps:
// enough to go back a few changes, without one more file in the folder for
// every run that touched it (six copies of topgrade.d/omega.toml).
const keepBackups = 3

// pruneBackups removes the oldest .omega-bak-<time> copies of path beyond
// the newest keep. The copies are named after the time they were made, so
// their names sort in age order. Only regular files that match exactly are
// touched; a failure to remove one is not an error for the write.
func pruneBackups(path string, keep int) {
	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	prefix := base + ".omega-bak-"
	var copies []string
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() || !strings.HasPrefix(name, prefix) || !backupStamp(strings.TrimPrefix(name, prefix)) {
			continue
		}
		copies = append(copies, name)
	}
	if len(copies) <= keep {
		return
	}
	sort.Slice(copies, func(i, j int) bool { return backupOrder(copies[i]) < backupOrder(copies[j]) })
	for _, name := range copies[:len(copies)-keep] {
		os.Remove(filepath.Join(dir, name))
	}
}

// backupStamp reports whether s is a backup's time stamp: 20060102-150405,
// optionally followed by -N for a second copy in the same second.
func backupStamp(s string) bool {
	if len(s) < 15 || s[8] != '-' {
		return false
	}
	for i, r := range s {
		if i == 8 || (i == 15 && r == '-') {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) == 15 || (len(s) > 16 && s[15] == '-')
}

// backupOrder makes "…-150405-10" sort after "…-150405-2".
func backupOrder(name string) string {
	stamp := name[strings.LastIndex(name, ".omega-bak-")+len(".omega-bak-"):]
	n := ""
	if len(stamp) > 16 {
		n = stamp[16:]
	}
	return stamp[:15] + strings.Repeat("0", 6-min(len(n), 6)) + n
}
