package cleaner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		// Without a bundle only what the user wrote counts, and never
		// Apple's identifiers.
		{"com.apple.dt.Xcode.plist", "Xcode", "", false},
		{"com.apple.dt.Xcode.plist", "Xcode", "com.apple.dt.Xcode", true},
		{"com.apple.mail", "Mail", "", false},
		{"com.figma.Desktop", "Desktop", "", false},
		{"com.github.desktop.plist", "Desktop", "", false},
		{"Desktop", "Desktop", "", true},
		{"com.figma.Desktop", "com.figma.Desktop", "", true},
		{"com.figma.Desktop.plist", "com.figma.desktop", "", true},
		{"com.figma.DesktopHelper", "com.figma.Desktop", "", false},
		{"com.apple.mail", "com.apple.mail", "", false},
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
	oldOS, oldDirs, oldID, oldSys := goos, appDirs, bundleID, systemAppDirs
	defer func() { goos, appDirs, bundleID, systemAppDirs = oldOS, oldDirs, oldID, oldSys }()
	goos = "darwin"
	systemAppDirs = nil
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

// A file edited deep in the project makes it active: page.tsx sits four
// folders down.
func TestFindArtifactsSeesDeepEdits(t *testing.T) {
	tmp := t.TempDir()
	proj := filepath.Join(tmp, "app")
	nm := filepath.Join(proj, "node_modules")
	write(t, filepath.Join(nm, "dep", "index.js"))
	write(t, filepath.Join(proj, "package.json"))
	deep := filepath.Join(proj, "src", "app", "dashboard", "page.tsx")
	write(t, deep)
	age(t, nm, filepath.Join(proj, "package.json"), proj, filepath.Join(proj, "src"),
		filepath.Join(proj, "src", "app"), filepath.Join(proj, "src", "app", "dashboard"))
	// page.tsx was written just now.

	if found := FindArtifacts([]string{tmp}, 30); len(found) != 0 {
		t.Fatalf("a project edited four folders down was listed: %+v", found)
	}
	age(t, deep)
	if found := FindArtifacts([]string{tmp}, 30); len(found) != 1 {
		t.Fatalf("the old project was not listed: %+v", found)
	}
}

// A project too large to read whole is taken for active, not for old.
func TestFindArtifactsLargeProjectIsActive(t *testing.T) {
	old := activityCap
	activityCap = 50
	defer func() { activityCap = old }()
	tmp := t.TempDir()
	proj := filepath.Join(tmp, "project")
	nm := filepath.Join(proj, "node_modules")
	write(t, filepath.Join(nm, "dep", "index.js"))
	write(t, filepath.Join(proj, "package.json"))
	for i := 0; i < 60; i++ {
		f := filepath.Join(proj, "assets", fmt.Sprintf("img%03d.png", i))
		write(t, f)
		age(t, f)
	}
	write(t, filepath.Join(proj, "src", "main.js")) // written just now, after assets/ in name order
	age(t, nm, filepath.Join(proj, "package.json"), filepath.Join(proj, "assets"), filepath.Join(proj, "src"), proj)

	if found := FindArtifacts([]string{tmp}, 30); len(found) != 0 {
		t.Fatalf("a project larger than the cap was listed: %+v", found)
	}
}

// A root that is a link (~/Dev often is) is searched like any other.
func TestFindArtifactsFollowsALinkedRoot(t *testing.T) {
	tmp := t.TempDir()
	proj := filepath.Join(tmp, "real", "project")
	nm := filepath.Join(proj, "node_modules")
	write(t, filepath.Join(nm, "dep", "index.js"))
	write(t, filepath.Join(proj, "package.json"))
	age(t, nm, filepath.Join(proj, "package.json"), proj)
	link := filepath.Join(tmp, "Dev")
	if err := os.Symlink(filepath.Join(tmp, "real"), link); err != nil {
		t.Skip("links are not available here: " + err.Error())
	}
	found := FindArtifacts([]string{link}, 30)
	if len(found) != 1 || found[0].Path != filepath.Join(link, "project", "node_modules") {
		t.Fatalf("through the link: %+v", found)
	}
}

// In a repository git has the last word: a commit made today makes the
// project active whatever the dates of the files, and looking does not
// touch .git/index.
func TestFindArtifactsAsksGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	tmp := t.TempDir()
	proj := filepath.Join(tmp, "project")
	nm := filepath.Join(proj, "node_modules")
	write(t, filepath.Join(nm, "dep", "index.js"))
	write(t, filepath.Join(proj, "package.json"))
	if err := os.WriteFile(filepath.Join(proj, ".gitignore"), []byte("node_modules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git := func(date time.Time, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", proj, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		stamp := date.Format(time.RFC3339)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+stamp, "GIT_COMMITTER_DATE="+stamp)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	ageAll := func() {
		t.Helper()
		var all []string
		filepath.Walk(proj, func(p string, _ os.FileInfo, err error) error {
			if err == nil {
				all = append(all, p)
			}
			return nil
		})
		age(t, all...)
	}
	long := time.Now().AddDate(0, 0, -60)
	git(long, "init", "-q")
	git(long, "add", "-A")
	git(long, "commit", "-q", "-m", "first")
	ageAll()
	index, _ := os.Stat(filepath.Join(proj, ".git", "index"))

	if found := FindArtifacts([]string{tmp}, 30); len(found) != 1 {
		t.Fatalf("the project last committed 60 days ago was not listed: %+v", found)
	}
	if after, _ := os.Stat(filepath.Join(proj, ".git", "index")); index != nil && after != nil && !after.ModTime().Equal(index.ModTime()) {
		t.Error("looking at the project rewrote .git/index")
	}

	write(t, filepath.Join(proj, "src", "main.js"))
	git(time.Now(), "add", "-A")
	git(time.Now(), "commit", "-q", "-m", "today")
	ageAll()
	if found := FindArtifacts([]string{tmp}, 30); len(found) != 0 {
		t.Fatalf("a project committed to today was listed: %+v", found)
	}
}

// Thirty files with one name from thirty folders, and twenty folders with
// one name: each keeps its place in the Trash and its content.
func TestRemoveNeverOverwrites(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // the home directory on Windows
	oldOS := goos
	defer func() { goos = oldOS }()
	goos = "darwin"
	trash := filepath.Join(home, ".Trash")

	seen := map[string]bool{}
	for i := 0; i < 30; i++ {
		p := filepath.Join(home, "lib", fmt.Sprint(i), "com.foo.Bar.plist")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(fmt.Sprint(i)), 0o644); err != nil {
			t.Fatal(err)
		}
		dest, err := Remove(p)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(dest)
		if err != nil || string(data) != fmt.Sprint(i) {
			t.Fatalf("%s holds %q, %v; want %d", dest, data, err, i)
		}
		if seen[dest] || filepath.Ext(dest) != ".plist" {
			t.Fatalf("destination %s: used twice or lost its extension", dest)
		}
		seen[dest] = true
	}
	for i := 0; i < 20; i++ {
		p := filepath.Join(home, "data", fmt.Sprint(i), "com.foo.Bar")
		write(t, filepath.Join(p, fmt.Sprint(i)))
		dest, err := Remove(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dest, fmt.Sprint(i))); err != nil {
			t.Fatalf("folder %d is not whole in the Trash: %v", i, err)
		}
	}
	entries, _ := os.ReadDir(trash)
	if len(entries) != 50 {
		t.Fatalf("the Trash holds %d entries, want 50", len(entries))
	}
	// The first of each kept its data too.
	if data, _ := os.ReadFile(filepath.Join(trash, "com.foo.Bar.plist")); string(data) != "0" {
		t.Errorf("the first file was replaced: %q", data)
	}
}

// On Linux the trash is where XDG_DATA_HOME says, and the path in the
// .trashinfo is escaped as the specification asks.
func TestRemoveLinuxTrash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg"))
	oldOS := goos
	defer func() { goos = oldOS }()
	goos = "linux"
	p := filepath.Join(home, "my docs", "a file.txt")
	write(t, p)
	dest, err := Remove(p)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "xdg", "Trash", "files", "a file.txt"); dest != want {
		t.Errorf("moved to %s, want %s", dest, want)
	}
	info, err := os.ReadFile(filepath.Join(home, "xdg", "Trash", "info", "a file.txt.trashinfo"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(info), "/my%20docs/a%20file.txt\n") || strings.Contains(string(info), "a file") {
		t.Errorf("trashinfo: %s", info)
	}
}

// What comes with macOS is refused, and an app that is not installed gets
// nothing of Apple or of a vendor whose identifier ends in its name.
func TestFindAppArtifactsLeavesSystemAndOtherVendorsAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	oldOS, oldDirs, oldID, oldSys := goos, appDirs, bundleID, systemAppDirs
	defer func() { goos, appDirs, bundleID, systemAppDirs = oldOS, oldDirs, oldID, oldSys }()
	goos = "darwin"
	apps, system := filepath.Join(home, "Applications"), filepath.Join(home, "System", "Applications")
	appDirs = func(string) []string { return []string{apps} }
	systemAppDirs = []string{system}
	bundleID = func(string) string { return "" }
	for _, p := range []string{
		filepath.Join(system, "Mail.app", "x"),
		filepath.Join(home, "Library", "Containers", "com.apple.mail", "x"),
		filepath.Join(home, "Library", "Caches", "com.apple.mail", "x"),
		filepath.Join(home, "Library", "Application Support", "com.figma.Desktop", "x"),
		filepath.Join(home, "Library", "Preferences", "com.github.desktop.plist"),
		filepath.Join(home, "Library", "Preferences", "com.apple.desktop.plist"),
	} {
		write(t, p)
	}
	if _, err := FindAppArtifacts("mail"); !errors.Is(err, ErrSystemApp) {
		t.Errorf("Mail: %v, want ErrSystemApp", err)
	}
	app, err := FindAppArtifacts("Desktop")
	if err != nil {
		t.Fatal(err)
	}
	if len(app.DataPaths) != 0 {
		t.Errorf("Desktop took the data of other applications: %v", app.DataPaths)
	}
	// The whole identifier names one vendor's application.
	app, err = FindAppArtifacts("com.figma.Desktop")
	if err != nil || len(app.DataPaths) != 1 {
		t.Errorf("com.figma.Desktop: %v, %v", app, err)
	}
	// Safari lives in /Applications and is still the system's.
	write(t, filepath.Join(apps, "Safari.app", "x"))
	bundleID = func(string) string { return "com.apple.Safari" }
	if _, err := FindAppArtifacts("Safari"); !errors.Is(err, ErrSystemApp) {
		t.Errorf("Safari: %v, want ErrSystemApp", err)
	}
}
