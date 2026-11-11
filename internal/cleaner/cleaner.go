// Package cleaner provides deep cleanup tools: multi-language build artifact
// discovery and pruning (node_modules, target, .venv), duplicate file detection,
// deep app uninstallation, IDE cache hygiene, and container/cloud cache clearing.
package cleaner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
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
	AppName     string   `json:"app_name"`
	AppBundle   string   `json:"app_bundle,omitempty"`
	DataPaths   []string `json:"data_paths"`
	TotalSizeMB float64  `json:"total_size_mb"`
}

// LargeFile holds details for a large file.
type LargeFile struct {
	Path   string  `json:"path"`
	SizeMB float64 `json:"size_mb"`
}

// FindArtifacts scans given roots for project artifacts older than daysOld.
func FindArtifacts(roots []string, daysOld int) []ArtifactDir {
	cutoff := time.Now().AddDate(0, 0, -daysOld)
	artifactNames := map[string]string{
		"node_modules":  "Node.js",
		"target":        "Rust/Java",
		".venv":         "Python",
		"venv":          "Python",
		".pytest_cache": "Python Test",
		".ruff_cache":   "Python Ruff",
		".mypy_cache":   "Python Mypy",
		"__pycache__":   "Python Bytecode",
		".gradle":       "Gradle",
		"build":         "Build Output",
		"DerivedData":   "Xcode",
		".next":         "Next.js",
		".nuxt":         "Nuxt.js",
		".turbo":        "Turborepo",
	}

	var results []ArtifactDir

	for _, root := range roots {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || !info.IsDir() {
				return nil
			}
			name := info.Name()
			if kind, ok := artifactNames[name]; ok {
				modTime := info.ModTime()
				if modTime.Before(cutoff) {
					sizeMB := dirSizeMB(path)
					days := int(time.Since(modTime).Hours() / 24)
					results = append(results, ArtifactDir{
						Path:    path,
						Type:    kind + " (" + name + ")",
						SizeMB:  sizeMB,
						ModTime: modTime,
						DaysOld: days,
					})
				}
				return filepath.SkipDir // Do not descend inside artifact directory
			}
			return nil
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

// FindAppArtifacts locates application files and leftover directories for clean uninstallation.
func FindAppArtifacts(appName string) (*AppArtifacts, error) {
	home, _ := os.UserHomeDir()
	app := &AppArtifacts{AppName: appName}
	appNameLower := strings.ToLower(appName)

	if goos == "darwin" {
		// Look in /Applications and ~/Applications
		for _, dir := range []string{"/Applications", filepath.Join(home, "Applications")} {
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".app") {
					base := strings.TrimSuffix(e.Name(), ".app")
					if strings.EqualFold(base, appName) || strings.Contains(strings.ToLower(base), appNameLower) {
						app.AppBundle = filepath.Join(dir, e.Name())
						break
					}
				}
			}
		}

		// Search library directories
		searchDirs := []string{
			filepath.Join(home, "Library/Application Support"),
			filepath.Join(home, "Library/Caches"),
			filepath.Join(home, "Library/Preferences"),
			filepath.Join(home, "Library/Saved Application State"),
			filepath.Join(home, "Library/Logs"),
			filepath.Join(home, "Library/Containers"),
		}

		for _, sDir := range searchDirs {
			entries, _ := os.ReadDir(sDir)
			for _, e := range entries {
				lower := strings.ToLower(e.Name())
				if strings.Contains(lower, appNameLower) {
					p := filepath.Join(sDir, e.Name())
					app.DataPaths = append(app.DataPaths, p)
					app.TotalSizeMB += float64(dirSizeMB(p))
				}
			}
		}
	} else {
		// Linux / Windows path patterns
		var searchDirs []string
		if goos == "windows" {
			searchDirs = []string{
				filepath.Join(home, "AppData/Roaming"),
				filepath.Join(home, "AppData/Local"),
			}
		} else {
			searchDirs = []string{
				filepath.Join(home, ".config"),
				filepath.Join(home, ".local/share"),
				filepath.Join(home, ".cache"),
			}
		}
		for _, sDir := range searchDirs {
			entries, _ := os.ReadDir(sDir)
			for _, e := range entries {
				if strings.Contains(strings.ToLower(e.Name()), appNameLower) {
					p := filepath.Join(sDir, e.Name())
					app.DataPaths = append(app.DataPaths, p)
					app.TotalSizeMB += float64(dirSizeMB(p))
				}
			}
		}
	}

	return app, nil
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
