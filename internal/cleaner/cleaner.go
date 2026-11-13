// Package cleaner provides deep cleanup tools: multi-language build artifact
// discovery and pruning (node_modules, target, .venv), duplicate file detection
// and deep app uninstallation.
package cleaner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

var goos = runtime.GOOS

// ArtifactDir represents an identified build or dependency artifact folder.
type ArtifactDir struct {
	Path    string    `json:"path"`
	Type    string    `json:"type"` // "node_modules", "target", ".venv", etc.
	SizeMB  int64     `json:"size_mb"`
	ModTime time.Time `json:"mod_time"`
	DaysOld int       `json:"days_old"`
}

// DuplicateGroup represents a group of identical files sharing the same SHA-256 hash.
type DuplicateGroup struct {
	Hash   string   `json:"hash"`
	SizeMB float64  `json:"size_mb"`
	Paths  []string `json:"paths"`
}

// AppArtifacts holds all files and directories discovered for an application.
type AppArtifacts struct {
	AppName   string   `json:"app_name"`
	AppBundle string   `json:"app_bundle,omitempty"`
	DataPaths []string `json:"data_paths"`
	// Similar are installed applications whose name contains the one asked
	// for. They are not selected.
	Similar     []string `json:"similar,omitempty"`
	TotalSizeMB float64  `json:"total_size_mb"`
}

// LargeFile holds details for a large file.
type LargeFile struct {
	Path   string  `json:"path"`
	SizeMB float64 `json:"size_mb"`
}

// artifactRule says what a folder name is taken for, and what has to be true
// before it is: a name alone is not enough to delete a folder. "build" and
// "target" are ordinary words (a branch build/x is the folder
// .git/refs/heads/build, and plenty of projects keep sources under build/).
type artifactRule struct {
	kind string
	// beside lists files of which one must sit next to the folder, in the
	// project that produces it. inside lists files of which one must be in
	// it. With both empty the name is specific enough.
	beside []string
	inside []string
}

var artifactRules = map[string]artifactRule{
	"node_modules":  {kind: "Node.js", beside: []string{"package.json"}},
	"target":        {kind: "Rust/Java", beside: []string{"Cargo.toml", "pom.xml", "build.sbt"}},
	".venv":         {kind: "Python", inside: []string{"pyvenv.cfg"}},
	"venv":          {kind: "Python", inside: []string{"pyvenv.cfg"}},
	".pytest_cache": {kind: "Python Test"},
	".ruff_cache":   {kind: "Python Ruff"},
	".mypy_cache":   {kind: "Python Mypy"},
	"__pycache__":   {kind: "Python Bytecode"},
	".gradle":       {kind: "Gradle", beside: []string{"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts", "gradlew"}},
	"build":         {kind: "Build Output", beside: []string{"build.gradle", "build.gradle.kts"}, inside: []string{"CMakeCache.txt"}},
	"DerivedData":   {kind: "Xcode", inside: []string{"info.plist", "ModuleCache.noindex"}},
	".next":         {kind: "Next.js", beside: []string{"package.json"}},
	".nuxt":         {kind: "Nuxt.js", beside: []string{"package.json"}},
	".turbo":        {kind: "Turborepo", beside: []string{"package.json"}},
}

// matches reports whether the folder at path is what the rule describes.
func (r artifactRule) matches(path string) bool {
	if len(r.beside) == 0 && len(r.inside) == 0 {
		return true
	}
	for _, f := range r.beside {
		if fi, err := os.Stat(filepath.Join(filepath.Dir(path), f)); err == nil && !fi.IsDir() {
			return true
		}
	}
	for _, f := range r.inside {
		if _, err := os.Stat(filepath.Join(path, f)); err == nil {
			return true
		}
	}
	return false
}

// tracked reports whether git keeps files of the folder: then it is part of
// the project, not something a build can make again.
func tracked(path string) bool {
	if _, err := exec.LookPath("git"); err != nil {
		return false
	}
	cmd := exec.Command("git", "-C", filepath.Dir(path), "ls-files", "--", filepath.Base(path))
	cmd.Stderr = nil
	out, err := cmd.Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

// activityCap is how many entries of a project are read to date it. A
// project with more is taken for active: the walk goes in name order, and
// stopping early would miss src/ after a large assets/.
var activityCap = 20000

// gitOutput runs git in dir and returns what it printed. ok is false when
// git is missing, dir is not in a repository or git took too long.
func gitOutput(dir string, args ...string) (out []byte, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	// Looking must not write: git status refreshes .git/index otherwise,
	// and the next scan would take that for work on the project.
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.Output()
	return out, err == nil
}

// lastActivity is when the project that owns an artifact folder was last
// worked on. The folder's own date does not say: installing a dependency
// months ago and editing sources today leaves node_modules old. So it is
// the newest date among the folder, the project's files and folders (other
// artifact folders left out), what git touches on every commit, checkout
// or fetch, the last commit of the project and the files git reports as
// changed. When that cannot be established the answer is now: the project
// counts as active.
func lastActivity(artifact string, info os.FileInfo) time.Time {
	newest := info.ModTime()
	see := func(t time.Time) {
		if t.After(newest) {
			newest = t
		}
	}
	project := filepath.Dir(artifact)
	for _, f := range []string{".git/index", ".git/HEAD", ".git/logs/HEAD", ".git/FETCH_HEAD"} {
		if fi, err := os.Stat(filepath.Join(project, f)); err == nil {
			see(fi.ModTime())
		}
	}
	if top, ok := gitOutput(project, "rev-parse", "--show-toplevel"); ok {
		root := strings.TrimSpace(string(top))
		if out, ok := gitOutput(project, "log", "-1", "--format=%ct", "--", "."); ok {
			if sec, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); err == nil {
				see(time.Unix(sec, 0))
			}
		}
		out, ok := gitOutput(project, "status", "--porcelain", "-z", "--", ".")
		if !ok {
			return time.Now()
		}
		// Each entry is "XY path"; a rename is followed by the old path
		// alone. A changed file counts by its date, a deleted one by the
		// date of the folder it was in.
		for _, entry := range bytes.Split(out, []byte{0}) {
			name := string(entry)
			if len(name) > 3 && name[2] == ' ' {
				name = name[3:]
			}
			if name == "" {
				continue
			}
			path := filepath.Join(root, filepath.FromSlash(name))
			if _, isArtifact := artifactRules[filepath.Base(path)]; isArtifact {
				continue // the artifact folder itself, not ignored by git
			}
			if fi, err := os.Lstat(path); err == nil {
				see(fi.ModTime())
			} else if fi, err := os.Lstat(filepath.Dir(path)); err == nil {
				see(fi.ModTime())
			}
		}
	}
	seen := 0
	capped := false
	_ = filepath.Walk(project, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if seen++; seen > activityCap {
			capped = true
			return filepath.SkipAll
		}
		if fi.IsDir() && path != project {
			if _, ok := artifactRules[fi.Name()]; ok || fi.Name() == ".git" {
				return filepath.SkipDir
			}
		}
		see(fi.ModTime())
		return nil
	})
	if capped {
		return time.Now()
	}
	return newest
}

// FindArtifacts scans given roots for build and dependency folders of
// projects that have not been worked on for daysOld days. A folder counts
// only when its project is recognisable (see artifactRules), git does not
// track its files, and it is not inside a .git folder.
func FindArtifacts(roots []string, daysOld int) []ArtifactDir {
	cutoff := time.Now().AddDate(0, 0, -daysOld)
	var results []ArtifactDir

	for _, root := range roots {
		// Walk does not follow a link, and ~/Dev is often one. What is
		// found is still reported under the root as it was given.
		real, err := filepath.EvalSymlinks(root)
		if err != nil {
			real = root
		}
		_ = filepath.Walk(real, func(path string, info os.FileInfo, err error) error {
			if err != nil || !info.IsDir() {
				return nil
			}
			name := info.Name()
			if name == ".git" {
				return filepath.SkipDir
			}
			rule, ok := artifactRules[name]
			if !ok || path == real {
				return nil
			}
			if !rule.matches(path) {
				// A folder that only shares the name: look inside it like
				// any other, there may be projects below.
				return nil
			}
			if tracked(path) {
				return filepath.SkipDir
			}
			if active := lastActivity(path, info); active.Before(cutoff) {
				shown := path
				if rel, err := filepath.Rel(real, path); err == nil {
					shown = filepath.Join(root, rel)
				}
				results = append(results, ArtifactDir{
					Path:    shown,
					Type:    rule.kind + " (" + name + ")",
					SizeMB:  dirSizeMB(path),
					ModTime: active,
					DaysOld: int(time.Since(active).Hours() / 24),
				})
			}
			return filepath.SkipDir // Do not descend inside artifact directory
		})
	}

	// Sort largest first
	sort.Slice(results, func(i, j int) bool {
		return results[i].SizeMB > results[j].SizeMB
	})

	return results
}

// PruneArtifact removes the specified artifact directory.
func PruneArtifact(path string) error {
	return os.RemoveAll(path)
}

// FindDuplicates scans dir and detects identical files based on size and SHA-256 hash.
func FindDuplicates(dir string, minSizeMB int64) ([]DuplicateGroup, error) {
	if minSizeMB <= 0 {
		minSizeMB = 1 // default minimum 1MB to avoid hashing tiny files
	}
	minBytes := minSizeMB * 1024 * 1024

	bySize := make(map[int64][]string)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if info.Size() >= minBytes {
			bySize[info.Size()] = append(bySize[info.Size()], path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var duplicates []DuplicateGroup

	// Hash only files with duplicate sizes
	for size, files := range bySize {
		if len(files) < 2 {
			continue
		}
		byHash := make(map[string][]string)
		for _, f := range files {
			h, err := hashFile(f)
			if err == nil {
				byHash[h] = append(byHash[h], f)
			}
		}
		for h, matching := range byHash {
			if len(matching) > 1 {
				duplicates = append(duplicates, DuplicateGroup{
					Hash:   h,
					SizeMB: float64(size) / (1024 * 1024),
					Paths:  matching,
				})
			}
		}
	}

	sort.Slice(duplicates, func(i, j int) bool {
		return duplicates[i].SizeMB > duplicates[j].SizeMB
	})

	return duplicates, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// FindLargeFiles returns files larger than minSizeMB in dir.
func FindLargeFiles(dir string, minSizeMB int64, limit int) ([]LargeFile, error) {
	if minSizeMB <= 0 {
		minSizeMB = 100
	}
	if limit <= 0 {
		limit = 20
	}
	minBytes := minSizeMB * 1024 * 1024

	var files []LargeFile
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if info.Size() >= minBytes {
			files = append(files, LargeFile{
				Path:   path,
				SizeMB: float64(info.Size()) / (1024 * 1024),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].SizeMB > files[j].SizeMB
	})

	if len(files) > limit {
		files = files[:limit]
	}
	return files, nil
}

// ErrAppName is returned for a name too short to stand for one application.
var ErrAppName = errors.New("the application name needs at least two characters")

// ErrSystemApp is returned for an application that is part of macOS.
var ErrSystemApp = errors.New("is part of macOS and cannot be uninstalled")

// bundleID reads CFBundleIdentifier from an app bundle.
var bundleID = func(bundle string) string {
	out, err := exec.Command("/usr/bin/defaults", "read", filepath.Join(bundle, "Contents", "Info"), "CFBundleIdentifier").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// appDirs are where applications the user installed are looked up on a Mac.
var appDirs = func(home string) []string {
	return []string{"/Applications", filepath.Join(home, "Applications")}
}

// systemAppDirs are where the applications that come with macOS are.
var systemAppDirs = []string{"/System/Applications", "/System/Applications/Utilities"}

// appleID reports whether an identifier (or a file named after one) is
// Apple's.
func appleID(s string) bool { return strings.HasPrefix(strings.ToLower(s), "com.apple.") }

// looksLikeID reports whether a name is written as a bundle identifier.
func looksLikeID(s string) bool { return strings.Count(s, ".") >= 2 }

// ownedBy reports whether an entry of a Library (or config) folder belongs
// to the application. The whole name has to be the application's name or
// its bundle identifier, optionally followed by a dot and more
// ("com.foo.Bar.plist", "com.foo.Bar.helper"). Part of a name is not
// enough: "Code" is not "Xcode", and "Arc" is not "Search".
//
// id is read from the installed bundle. Without one (the app is already
// gone) only what the user wrote counts, a folder name or a whole
// identifier: taking any identifier that ends in the name gave Mail the
// data of com.apple.mail and Desktop that of com.figma.Desktop. Apple's
// identifiers are matched only through an installed bundle.
func ownedBy(entry, appName, id string) bool {
	lower := strings.ToLower(entry)
	stem := lower
	for _, ext := range []string{".plist", ".savedstate", ".binarycookies"} {
		stem = strings.TrimSuffix(stem, ext)
	}
	name := strings.ToLower(appName)
	if id == "" && looksLikeID(name) {
		id = name
		if appleID(id) {
			return false
		}
	}
	if id != "" {
		id = strings.ToLower(id)
		if stem == id || strings.HasPrefix(lower, id+".") {
			return true
		}
	}
	return stem == name && !appleID(stem)
}

// FindAppArtifacts locates one application and what it left in the user's
// folders. The application is found by its exact name (case does not
// matter); Similar lists the installed ones that only contain the name, and
// nothing of those is selected.
func FindAppArtifacts(appName string) (*AppArtifacts, error) {
	appName = strings.TrimSuffix(strings.TrimSpace(appName), ".app")
	if len([]rune(appName)) < 2 || strings.ContainsAny(appName, `/\`) {
		return nil, ErrAppName
	}
	home, _ := os.UserHomeDir()
	app := &AppArtifacts{AppName: appName}
	appNameLower := strings.ToLower(appName)
	id := ""

	var searchDirs []string
	switch goos {
	case "darwin":
		for _, dir := range systemAppDirs {
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if strings.EqualFold(e.Name(), appName+".app") {
					return nil, fmt.Errorf("%s %w", strings.TrimSuffix(e.Name(), ".app"), ErrSystemApp)
				}
			}
		}
		for _, dir := range appDirs(home) {
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if !strings.HasSuffix(e.Name(), ".app") {
					continue
				}
				base := strings.TrimSuffix(e.Name(), ".app")
				switch {
				case strings.EqualFold(base, appName):
					if app.AppBundle == "" {
						app.AppBundle = filepath.Join(dir, e.Name())
					}
				case strings.Contains(strings.ToLower(base), appNameLower):
					app.Similar = append(app.Similar, base)
				}
			}
		}
		if app.AppBundle != "" {
			id = bundleID(app.AppBundle)
			// Safari is the one system app kept in /Applications.
			if strings.EqualFold(id, "com.apple.Safari") {
				return nil, fmt.Errorf("%s %w", appName, ErrSystemApp)
			}
			app.TotalSizeMB += float64(dirSizeMB(app.AppBundle))
		}
		for _, d := range []string{"Application Support", "Caches", "Preferences", "Saved Application State", "Logs", "Containers", "HTTPStorages", "WebKit"} {
			searchDirs = append(searchDirs, filepath.Join(home, "Library", d))
		}
	case "windows":
		searchDirs = []string{
			filepath.Join(home, "AppData/Roaming"),
			filepath.Join(home, "AppData/Local"),
		}
	default:
		searchDirs = []string{
			filepath.Join(home, ".config"),
			filepath.Join(home, ".local/share"),
			filepath.Join(home, ".cache"),
		}
	}

	for _, sDir := range searchDirs {
		entries, _ := os.ReadDir(sDir)
		for _, e := range entries {
			if ownedBy(e.Name(), appName, id) {
				p := filepath.Join(sDir, e.Name())
				app.DataPaths = append(app.DataPaths, p)
				app.TotalSizeMB += float64(dirSizeMB(p))
			}
		}
	}

	return app, nil
}

// trashDir is where removed things go, or "" where there is no such place.
func trashDir() string {
	home, _ := os.UserHomeDir()
	switch goos {
	case "darwin":
		return filepath.Join(home, ".Trash")
	case "windows":
		return ""
	}
	// The trash of the desktop, where its specification puts it.
	data := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(data) {
		data = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(data, "Trash", "files")
}

// Remove takes a file or a folder away without deleting it where the system
// has a place for that: the Trash on a Mac, the trash folder of the desktop
// on Linux. It returns where it went, or "" when it was deleted (Windows,
// where the Recycle Bin cannot be reached from here).
func Remove(path string) (string, error) {
	dir := trashDir()
	if dir == "" {
		return "", os.RemoveAll(path)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	// Never copy and delete: when the Trash is on another volume the move
	// fails and the files stay where they are.
	dest, err := moveUnique(path, dir)
	if err != nil {
		return "", err
	}
	if goos != "darwin" {
		info := filepath.Join(filepath.Dir(dir), "info")
		if os.MkdirAll(info, 0o700) == nil {
			abs, err := filepath.Abs(path)
			if err != nil {
				abs = path
			}
			// The specification wants the path escaped as in a URL.
			body := "[Trash Info]\nPath=" + (&url.URL{Path: filepath.ToSlash(abs)}).EscapedPath() +
				"\nDeletionDate=" + time.Now().Format("2006-01-02T15:04:05") + "\n"
			_ = os.WriteFile(filepath.Join(info, filepath.Base(dest)+".trashinfo"), []byte(body), 0o600)
		}
	}
	return dest, nil
}

// moveUnique moves path into dir under a name nothing there has: its own,
// or with " 2", " 3"… added. It never replaces what is in dir. os.Rename
// alone would: it puts a file over a file of the same name without a word.
func moveUnique(path, dir string) (string, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	base := filepath.Base(path)
	stem, ext := base, ""
	// The number goes before the extension of a file and of an app bundle;
	// other folders have dots in their names that are not extensions.
	if !fi.IsDir() || strings.HasSuffix(base, ".app") {
		ext = filepath.Ext(base)
		stem = strings.TrimSuffix(base, ext)
		if stem == "" {
			stem, ext = base, ""
		}
	}
	for n := 1; n < 100000; n++ {
		dest := filepath.Join(dir, base)
		if n > 1 {
			dest = filepath.Join(dir, fmt.Sprintf("%s %d%s", stem, n, ext))
		}
		if fi.IsDir() {
			// A folder cannot be moved over a file or a folder with
			// something in it: the move fails and the next name is tried.
			if _, err := os.Lstat(dest); err == nil {
				continue
			}
			err := os.Rename(path, dest)
			if err == nil {
				return dest, nil
			}
			if _, statErr := os.Lstat(path); statErr != nil {
				return "", err
			}
			if _, statErr := os.Lstat(dest); statErr == nil {
				continue // taken in the meantime
			}
			return "", err
		}
		// A file: hold the name with an empty file made by this call, which
		// fails when the name is taken, and move over that.
		hold, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		hold.Close()
		if err := os.Rename(path, dest); err != nil {
			os.Remove(dest)
			return "", err
		}
		return dest, nil
	}
	return "", fmt.Errorf("no free name for %s in %s", base, dir)
}

func dirSizeMB(path string) int64 {
	var total int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total / (1024 * 1024)
}
