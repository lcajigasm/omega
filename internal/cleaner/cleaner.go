// Package cleaner provides deep cleanup tools: multi-language build artifact
// discovery and pruning (node_modules, target, .venv), duplicate file detection,
// deep app uninstallation, IDE cache hygiene, and container/cloud cache clearing.
package cleaner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/runner"
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

// lastActivity is when the project that owns an artifact folder was last
// worked on. The folder's own date does not say: installing a dependency
// months ago and editing sources today leaves node_modules old. So it is
// the newest date among the folder, the project's files (three levels,
// other artifact folders left out) and what git touches on every commit,
// checkout or fetch.
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
	seen := 0
	depth := strings.Count(project, string(filepath.Separator))
	_ = filepath.Walk(project, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if seen++; seen > 5000 {
			return filepath.SkipAll
		}
		if fi.IsDir() && path != project {
			if _, ok := artifactRules[fi.Name()]; ok || fi.Name() == ".git" {
				return filepath.SkipDir
			}
			if strings.Count(path, string(filepath.Separator))-depth >= 3 {
				return filepath.SkipDir
			}
		}
		see(fi.ModTime())
		return nil
	})
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
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || !info.IsDir() {
				return nil
			}
			name := info.Name()
			if name == ".git" {
				return filepath.SkipDir
			}
			rule, ok := artifactRules[name]
			if !ok || path == root {
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
				results = append(results, ArtifactDir{
					Path:    path,
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

// bundleID reads CFBundleIdentifier from an app bundle.
var bundleID = func(bundle string) string {
	out, err := exec.Command("/usr/bin/defaults", "read", filepath.Join(bundle, "Contents", "Info"), "CFBundleIdentifier").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// appDirs are where applications are looked up on a Mac.
var appDirs = func(home string) []string {
	return []string{"/Applications", filepath.Join(home, "Applications")}
}

// ownedBy reports whether an entry of a Library (or config) folder belongs
// to the application. The whole name has to be the application's name or
// its bundle identifier, optionally followed by a dot and more
// ("com.foo.Bar.plist", "com.foo.Bar.helper"). Part of a name is not
// enough: "Code" is not "Xcode", and "Arc" is not "Search".
func ownedBy(entry, appName, id string) bool {
	lower := strings.ToLower(entry)
	stem := lower
	for _, ext := range []string{".plist", ".savedstate", ".binarycookies"} {
		stem = strings.TrimSuffix(stem, ext)
	}
	name := strings.ToLower(appName)
	if stem == name {
		return true
	}
	if id != "" {
		id = strings.ToLower(id)
		return stem == id || strings.HasPrefix(lower, id+".")
	}
	// No bundle to read the identifier from (the app is already gone): an
	// identifier that ends in the name, as in com.apple.dt.Xcode.
	return strings.Count(stem, ".") >= 2 && strings.HasSuffix(stem, "."+name)
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

// Remove takes a file or a folder away without deleting it where the system
// has a place for that: the Trash on a Mac, the trash folder of the desktop
// on Linux. It returns where it went, or "" when it was deleted (Windows,
// where the Recycle Bin cannot be reached from here).
func Remove(path string) (string, error) {
	home, _ := os.UserHomeDir()
	var dir string
	switch goos {
	case "darwin":
		dir = filepath.Join(home, ".Trash")
	case "windows":
		return "", os.RemoveAll(path)
	default:
		dir = filepath.Join(home, ".local", "share", "Trash", "files")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, filepath.Base(path))
	if _, err := os.Lstat(dest); err == nil {
		ext := filepath.Ext(dest)
		dest = strings.TrimSuffix(dest, ext) + " " + time.Now().Format("2006-01-02 15.04.05.000") + ext
	}
	// Never copy and delete: when the Trash is on another volume the move
	// fails and the files stay where they are.
	if err := os.Rename(path, dest); err != nil {
		return "", err
	}
	if goos != "darwin" {
		info := filepath.Join(filepath.Dir(dir), "info")
		if os.MkdirAll(info, 0o700) == nil {
			body := "[Trash Info]\nPath=" + path + "\nDeletionDate=" + time.Now().Format("2006-01-02T15:04:05") + "\n"
			_ = os.WriteFile(filepath.Join(info, filepath.Base(dest)+".trashinfo"), []byte(body), 0o600)
		}
	}
	return dest, nil
}

// CleanIDECaches cleans JetBrains and VS Code workspace caches.
func CleanIDECaches(ctx context.Context) (int64, error) {
	home, _ := os.UserHomeDir()
	var paths []string

	if goos == "darwin" {
		paths = append(paths,
			filepath.Join(home, "Library/Application Support/Code/User/workspaceStorage"),
			filepath.Join(home, "Library/Application Support/Cursor/User/workspaceStorage"),
			filepath.Join(home, "Library/Caches/JetBrains"),
			filepath.Join(home, "Library/Developer/Xcode/Archives"),
		)
	} else if goos == "windows" {
		paths = append(paths,
			filepath.Join(home, "AppData/Roaming/Code/User/workspaceStorage"),
			filepath.Join(home, "AppData/Local/JetBrains"),
		)
	} else {
		paths = append(paths,
			filepath.Join(home, ".config/Code/User/workspaceStorage"),
			filepath.Join(home, ".cache/JetBrains"),
		)
	}

	var totalFreed int64
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			sz := dirSizeMB(p)
			if sz > 0 {
				_ = os.RemoveAll(p)
				_ = os.MkdirAll(p, 0755)
				totalFreed += sz
			}
		}
	}

	return totalFreed, nil
}

// CleanContainerAndCloudCaches cleans container logs, K8s caches, and cloud CLI caches.
func CleanContainerAndCloudCaches(ctx context.Context) (int64, error) {
	home, _ := os.UserHomeDir()
	var totalFreed int64

	// K8s & Cloud CLI caches
	targets := []string{
		filepath.Join(home, ".kube/cache"),
		filepath.Join(home, ".kube/http-cache"),
		filepath.Join(home, ".aws/cli/cache"),
		filepath.Join(home, ".azure/cache"),
	}

	for _, p := range targets {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			sz := dirSizeMB(p)
			if sz > 0 {
				_ = os.RemoveAll(p)
				totalFreed += sz
			}
		}
	}

	// Podman prune if available
	if runner.Has("podman") {
		_ = runner.Run(ctx, "podman system prune -a -f", 30*time.Second, nil)
	}

	return totalFreed, nil
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
