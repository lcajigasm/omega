package deepclean

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// scratchDepth is how deep under a root loose agent files are looked for:
// the root itself and the folders that hold the projects (~/Dev/work).
const scratchDepth = 2

// scratchFresh is how recent an agent file may be and still be offered as
// safe: a run of today may still be reading it.
const scratchFresh = 24 * time.Hour

// scanScratch finds the files agents' runs wrote outside any repository,
// next to the projects: transcripts piped to *.codex.txt, run records. One
// finding per folder.
func scanScratch(roots, repos []string, now time.Time) []Finding {
	isRepo := map[string]bool{}
	for _, r := range repos {
		isRepo[r] = true
	}
	var found []Finding
	seen := map[string]bool{}
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if seen[dir] || isRepo[dir] {
			return
		}
		seen[dir] = true
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return // a checkout: git status covers it
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		var files []string
		var size int64
		newest := time.Time{}
		for _, e := range entries {
			if e.IsDir() {
				if depth < scratchDepth && !strings.HasPrefix(e.Name(), ".") && e.Name() != "node_modules" {
					walk(filepath.Join(dir, e.Name()), depth+1)
				}
				continue
			}
			if !e.Type().IsRegular() || !IsAgentScratch(e.Name()) {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			files = append(files, e.Name())
			size += info.Size()
			if info.ModTime().After(newest) {
				newest = info.ModTime()
			}
		}
		if len(files) == 0 {
			return
		}
		sort.Strings(files)
		f := Finding{
			ID: id(KindAgentScratch, dir), Category: CatAgents, Kind: KindAgentScratch,
			Title: fmt.Sprintf("%d agent run files in %s", len(files), filepath.Base(dir)),
			Path:  dir, Size: size, Owner: owner(dir, files), LastUsed: newest,
			Reasons: []string{"transcripts and run records agents wrote next to the projects: " + list(files, 5),
				"newest " + ago(now.Sub(newest))},
			Action: fmt.Sprintf("move the %d files to the Trash", len(files)),
		}
		f.Verdict = Safe
		if now.Sub(newest) < scratchFresh {
			f.Verdict = Review
			f.Reasons = append(f.Reasons, "one was written in the last day: an agent may still be using it")
		}
		found = append(found, f)
	}
	for _, root := range roots {
		walk(root, 0)
	}
	return found
}
