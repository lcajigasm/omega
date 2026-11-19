package runner

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// A repository's own configuration can name programs that git runs while it
// only looks: a clean filter on "git status", a text converter on "git log
// -p", a pager, an alias. A clone never brings a .git/config; a folder that
// was unpacked or copied from somewhere else can. core.fsmonitor is
// switched off for every git omega runs (GitEnv); the rest cannot be
// switched off from outside (a filter has a name of its own, and an empty
// diff.external makes git diff fail), so such a repository is recognised
// instead.

// runsPrograms matches the settings of a .git/config that hold a command.
var runsPrograms = regexp.MustCompile(`(?im)^\s*\[\s*(filter|alias|pager|includeif|include)\b|` +
	`^\s*(textconv|command|clean|smudge|process|driver|sshcommand|pager|editor|hookspath|fsmonitor|external|program)\s*=`)

// GitConfigRuns reports whether the repository that holds dir has a
// configuration of its own that names a program for git to run. dir may be
// anywhere inside the work tree.
func GitConfigRuns(dir string) bool {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	for {
		dot := filepath.Join(dir, ".git")
		if info, err := os.Lstat(dot); err == nil {
			return gitDirRuns(dot, info, 0)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

func gitDirRuns(dot string, info os.FileInfo, depth int) bool {
	if depth > 2 {
		return true // a chain of pointers is not what git writes
	}
	if !info.IsDir() {
		// A work tree of another repository: ".git" is a file that says
		// where the real folder is.
		data, err := os.ReadFile(dot)
		if err != nil {
			return false
		}
		target, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
		if !ok {
			return true
		}
		target = strings.TrimSpace(target)
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(dot), target)
		}
		if _, err := os.Stat(target); err != nil {
			return false
		}
		if configRuns(filepath.Join(target, "config")) || configRuns(filepath.Join(target, "config.worktree")) {
			return true
		}
		// The shared part of the repository holds the main configuration.
		if common, err := os.ReadFile(filepath.Join(target, "commondir")); err == nil {
			c := strings.TrimSpace(string(common))
			if !filepath.IsAbs(c) {
				c = filepath.Join(target, c)
			}
			return configRuns(filepath.Join(c, "config"))
		}
		return false
	}
	return configRuns(filepath.Join(dot, "config")) || configRuns(filepath.Join(dot, "config.worktree"))
}

func configRuns(file string) bool {
	info, err := os.Stat(file)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if info.Size() > 1<<20 {
		return true
	}
	data, err := os.ReadFile(file)
	return err == nil && runsPrograms.Match(data)
}
