package assist

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The folders omega empties are always ones a definition names below a
// tool's own data folder; within reports whether p is such a place, so a
// mistake in a definition cannot reach the home folder itself.
func within(p, root string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

// emptyDirs removes what the folders hold and keeps the folders, which the
// app expects to find.
func emptyDirs(root string, dirs ...string) (int64, error) {
	var freed int64
	for _, d := range dirs {
		if !within(d, root) {
			return freed, fmt.Errorf("%s is outside %s: not touched", d, root)
		}
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			p := filepath.Join(d, e.Name())
			n := size(p)
			if err := os.RemoveAll(p); err != nil {
				return freed, err
			}
			freed += n
		}
	}
	return freed, nil
}

// removeOldest removes the oldest files under dirs until they hold no more
// than budget. Folders left empty go too.
func removeOldest(root string, budget int64, dirs ...string) (int64, error) {
	type file struct {
		path string
		size int64
		mod  int64
	}
	var files []file
	var total int64
	for _, d := range dirs {
		if !within(d, root) {
			return 0, fmt.Errorf("%s is outside %s: not touched", d, root)
		}
		filepath.Walk(d, func(p string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			n := onDisk(fi)
			files = append(files, file{p, n, fi.ModTime().UnixNano()})
			total += n
			return nil
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod < files[j].mod })
	var freed int64
	for _, f := range files {
		if total-freed <= budget {
			break
		}
		if err := os.Remove(f.path); err != nil {
			return freed, err
		}
		freed += f.size
		// Its folder goes when it is left empty (a day's sessions).
		for dir := filepath.Dir(f.path); within(dir, root) && !containsDir(dirs, dir); dir = filepath.Dir(dir) {
			if os.Remove(dir) != nil {
				break
			}
		}
	}
	return freed, nil
}

func containsDir(dirs []string, d string) bool {
	for _, x := range dirs {
		if filepath.Clean(x) == filepath.Clean(d) {
			return true
		}
	}
	return false
}

// oldVersions returns the entries of a folder of versions that are not in
// use: every one but those the links point into and the newest.
func oldVersions(dir string, links ...string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	keep := map[string]bool{}
	for _, l := range links {
		target, err := filepath.EvalSymlinks(l)
		if err != nil {
			continue
		}
		if rel, err := filepath.Rel(dir, target); err == nil && !strings.HasPrefix(rel, "..") {
			keep[strings.Split(rel, string(filepath.Separator))[0]] = true
		}
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			if rel, err := filepath.Rel(real, target); err == nil && !strings.HasPrefix(rel, "..") {
				keep[strings.Split(rel, string(filepath.Separator))[0]] = true
			}
		}
	}
	var names []string
	var newest string
	var newestMod int64
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		names = append(names, e.Name())
		if fi, err := e.Info(); err == nil && fi.ModTime().UnixNano() > newestMod {
			newest, newestMod = e.Name(), fi.ModTime().UnixNano()
		}
	}
	// Nothing a link points to was found: keep the newest, as the tool would
	// have started it.
	if len(keep) == 0 && newest != "" {
		keep[newest] = true
	}
	var out []string
	for _, n := range names {
		if !keep[n] {
			out = append(out, filepath.Join(dir, n))
		}
	}
	return out
}

// removeAll removes paths below root and says how much was freed.
func removeAll(root string, paths []string) (int64, error) {
	var freed int64
	for _, p := range paths {
		if !within(p, root) {
			return freed, fmt.Errorf("%s is outside %s: not touched", p, root)
		}
		n := size(p)
		if err := os.RemoveAll(p); err != nil {
			return freed, err
		}
		freed += n
	}
	return freed, nil
}

// electronCaches are the folders of a Chromium or Electron app's data that
// hold only caches: the app fills them again.
var electronCaches = []string{"Cache", "Code Cache", "GPUCache", "DawnGraphiteCache", "DawnWebGPUCache", "GraphiteDawnCache", "GPUPersistentCache", "ShaderCache", "GrShaderCache", "CachedData", "v8-code-cache", "Service Worker/CacheStorage"}

func cacheDirs(data string) []string {
	var out []string
	// A browser profile keeps them one level down.
	for _, base := range []string{data, filepath.Join(data, "Default")} {
		for _, c := range electronCaches {
			p := filepath.Join(base, c)
			if isDir(p) {
				out = append(out, p)
			}
		}
	}
	return out
}
