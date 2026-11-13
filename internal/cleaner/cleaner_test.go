package cleaner

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// age makes every path look untouched for 40 days.
func age(t *testing.T, paths ...string) {
	t.Helper()
	old := time.Now().AddDate(0, 0, -40)
	for _, p := range paths {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
}

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindArtifacts(t *testing.T) {
	tmp := t.TempDir()
	proj := filepath.Join(tmp, "project")
	nm := filepath.Join(proj, "node_modules")
	write(t, filepath.Join(nm, "dep", "index.js"))
	write(t, filepath.Join(proj, "package.json"))
	age(t, nm, filepath.Join(proj, "package.json"), proj)

	found := FindArtifacts([]string{tmp}, 30)
	if len(found) != 1 || found[0].Path != nm {
		t.Fatalf("expected node_modules of the old project, got %+v", found)
	}
}

// A project edited today keeps its dependencies, however old they are.
func TestFindArtifactsSkipsActiveProject(t *testing.T) {
	tmp := t.TempDir()
	proj := filepath.Join(tmp, "project")
	nm := filepath.Join(proj, "node_modules")
	write(t, filepath.Join(nm, "dep", "index.js"))
	write(t, filepath.Join(proj, "package.json"))
	write(t, filepath.Join(proj, "src", "main.js"))
	age(t, nm, filepath.Join(proj, "package.json"), filepath.Join(proj, "src"), proj)
	// src/main.js was written just now.

	if found := FindArtifacts([]string{tmp}, 30); len(found) != 0 {
		t.Fatalf("an active project lost its artifacts: %+v", found)
	}
}

// A name is not enough: git's folders for a branch build/x, and a source
// folder called build or target, are not build output.
func TestFindArtifactsNeedsAProject(t *testing.T) {
	tmp := t.TempDir()
	proj := filepath.Join(tmp, "project")
	paths := []string{
		filepath.Join(proj, ".git", "refs", "heads", "build"),
		filepath.Join(proj, ".git", "logs", "refs", "heads", "target"),
		filepath.Join(proj, "build"),
		filepath.Join(proj, "target"),
		filepath.Join(proj, "docs", "venv"),
		filepath.Join(proj, "node_modules"),
	}
	for _, p := range paths {
		write(t, filepath.Join(p, "file"))
		age(t, filepath.Join(p, "file"), p)
	}
	age(t, proj, filepath.Join(proj, "docs"))

	if found := FindArtifacts([]string{tmp}, 30); len(found) != 0 {
		t.Fatalf("folders that only share a name were taken for artifacts: %+v", found)
	}
}

func TestOwnedBy(t *testing.T) {
	cases := []struct {
		entry, app, id string
		want           bool
	}{
		{"Code", "Code", "com.microsoft.VSCode", true},
		{"com.microsoft.VSCode.plist", "Code", "com.microsoft.VSCode", true},
		{"com.microsoft.VSCode.helper", "Code", "com.microsoft.VSCode", true},
		{"Xcode", "Code", "com.microsoft.VSCode", false},
		{"com.apple.dt.Xcode", "Code", "com.microsoft.VSCode", false},
		{"com.apple.dt.Xcode.plist", "Code", "", false},
		{"CodexBar", "Code", "", false},
		{"Marcel", "Arc", "", false},
		{"Search", "Arc", "", false},
		{"com.apple.dt.Xcode.plist", "Xcode", "", true},
		{"com.microsoft.VSCodeInsiders", "Code", "com.microsoft.VSCode", false},
	}
	for _, c := range cases {
		if got := ownedBy(c.entry, c.app, c.id); got != c.want {
			t.Errorf("ownedBy(%q, %q, %q) = %v, want %v", c.entry, c.app, c.id, got, c.want)
		}
	}
}

func TestFindAppArtifactsExactName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // the home directory on Windows
	oldOS, oldDirs, oldID := goos, appDirs, bundleID
	defer func() { goos, appDirs, bundleID = oldOS, oldDirs, oldID }()
	goos = "darwin"
	apps := filepath.Join(home, "Applications")
	appDirs = func(string) []string { return []string{apps} }
	bundleID = func(string) string { return "com.microsoft.VSCode" }
	for _, p := range []string{
		filepath.Join(apps, "CodexBar.app", "x"),
		filepath.Join(apps, "Code.app", "x"),
		filepath.Join(apps, "Xcode.app", "x"),
		filepath.Join(home, "Library", "Application Support", "Code", "x"),
		filepath.Join(home, "Library", "Application Support", "Xcode", "x"),
		filepath.Join(home, "Library", "Caches", "com.apple.dt.Xcode", "x"),
		filepath.Join(home, "Library", "Preferences", "com.microsoft.VSCode.plist"),
	} {
		write(t, p)
	}

	app, err := FindAppArtifacts("code")
	if err != nil {
		t.Fatal(err)
	}
	if app.AppBundle != filepath.Join(apps, "Code.app") {
		t.Errorf("bundle = %q", app.AppBundle)
	}
	want := []string{
		filepath.Join(home, "Library", "Application Support", "Code"),
		filepath.Join(home, "Library", "Preferences", "com.microsoft.VSCode.plist"),
	}
	if len(app.DataPaths) != 2 || app.DataPaths[0] != want[0] || app.DataPaths[1] != want[1] {
		t.Errorf("data paths = %v, want %v", app.DataPaths, want)
	}
	if len(app.Similar) != 2 {
		t.Errorf("similar = %v", app.Similar)
	}

	for _, bad := range []string{"", " ", "a", "../x"} {
		if _, err := FindAppArtifacts(bad); err == nil {
			t.Errorf("FindAppArtifacts(%q) accepted the name", bad)
		}
	}
}

func TestRemoveGoesToTheTrash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // the home directory on Windows
	oldOS := goos
	defer func() { goos = oldOS }()
	goos = "darwin"
	for i := 0; i < 2; i++ {
		p := filepath.Join(home, "data", "thing")
		write(t, filepath.Join(p, "file"))
		dest, err := Remove(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dest, "file")); err != nil {
			t.Errorf("the file is not in the Trash: %v", err)
		}
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s is still there", p)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(home, ".Trash")); len(entries) != 2 {
		t.Errorf("the second removal overwrote the first: %d entries", len(entries))
	}
}

func TestFindDuplicates(t *testing.T) {
	tmp := t.TempDir()
	content := []byte(string(make([]byte, 1024*1024*2))) // 2MB
	f1 := filepath.Join(tmp, "file1.bin")
	f2 := filepath.Join(tmp, "file2.bin")
	_ = os.WriteFile(f1, content, 0644)
	_ = os.WriteFile(f2, content, 0644)

	dups, err := FindDuplicates(tmp, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(dups) != 1 {
		t.Fatalf("expected 1 duplicate group, got %d", len(dups))
	}
	if len(dups[0].Paths) != 2 {
		t.Errorf("expected 2 paths in group, got %d", len(dups[0].Paths))
	}
}

func TestFindLargeFiles(t *testing.T) {
	tmp := t.TempDir()
	content := []byte(string(make([]byte, 1024*1024*3))) // 3MB
	f1 := filepath.Join(tmp, "large.bin")
	_ = os.WriteFile(f1, content, 0644)

	files, err := FindLargeFiles(tmp, 2, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 large file, got %d", len(files))
	}
}
