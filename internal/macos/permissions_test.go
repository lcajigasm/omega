package macos

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
)

// fakeTCC serves canned privacy databases through Output.
func fakeTCC(t *testing.T, user, system string, userErr error) {
	t.Helper()
	oldOut, oldDBs, oldExists, oldFound := Output, tccDBs, pathExists, bundleFound
	t.Cleanup(func() { Output, tccDBs, pathExists, bundleFound = oldOut, oldDBs, oldExists, oldFound })
	tccDBs = func() []string { return []string{"/user/TCC.db", "/system/TCC.db"} }
	Output = func(name string, args ...string) ([]byte, error) {
		if name != "sqlite3" {
			return nil, errors.New("unexpected " + name)
		}
		for _, a := range args {
			switch a {
			case "/user/TCC.db":
				return []byte(user), userErr
			case "/system/TCC.db":
				return []byte(system), nil
			}
		}
		return nil, errors.New("no database")
	}
}

func row(fields ...string) string { return strings.Join(fields, "\x1f") + "\n" }

func TestPermissionsFindWhatIsLeftOver(t *testing.T) {
	user := row("kTCCServiceSystemPolicyDocumentsFolder", "/Users/me/Library/Application Support/Claude/claude-code/2.0.14/node", "1", "2", "UNUSED", "1760000000") +
		row("kTCCServiceSystemPolicyDocumentsFolder", "/Users/me/Library/Application Support/Claude/claude-code/2.0.31/node", "1", "2", "UNUSED", "1762000000") +
		row("kTCCServiceSystemPolicyDocumentsFolder", "/opt/homebrew/Cellar/node/24.1.0/bin/node", "1", "2", "UNUSED", "1761000000") +
		row("kTCCServiceAppleEvents", "com.example.Gone", "0", "2", "com.apple.finder", "1750000000") +
		row("kTCCServiceCamera", "com.example.Here", "0", "0", "UNUSED", "0") +
		row("kTCCServiceCamera", "com.apple.FaceTime", "0", "2", "UNUSED", "0")
	system := row("kTCCServiceSystemPolicyAllFiles", "com.example.Nested", "0", "2", "UNUSED", "0")
	fakeTCC(t, user, system, nil)
	pathExists = func(p string) bool { return strings.Contains(p, "2.0.31") || strings.Contains(p, "Cellar") }
	bundleFound = func(b string) bool { return b == "com.example.Nested" }

	perms, noAccess := Permissions(&UserPolicy{}, map[string]App{"com.example.Here": {Name: "Here", Bundle: "com.example.Here"}})
	if noAccess {
		t.Fatal("the user database was readable")
	}
	if len(perms) != 7 {
		t.Fatalf("got %d entries, want 7", len(perms))
	}
	by := map[string]Permission{}
	for _, p := range perms {
		by[p.Client] = p
	}
	old := by["/Users/me/Library/Application Support/Claude/claude-code/2.0.14/node"]
	if !old.Missing || old.Verdict != Disable || old.Why != WhyGone {
		t.Errorf("the node of a removed version should be offered for removal: %+v", old)
	}
	if old.Name != "node 2.0.14" || old.Copies != 3 {
		t.Errorf("name %q copies %d, want node 2.0.14 and 3", old.Name, old.Copies)
	}
	if cur := by["/Users/me/Library/Application Support/Claude/claude-code/2.0.31/node"]; cur.Verdict != Keep || cur.Missing {
		t.Errorf("a binary that exists must stay: %+v", cur)
	}
	if gone := by["com.example.Gone"]; gone.Verdict != Review || gone.Why != WhyNotInstalled || gone.Target != "com.apple.finder" || !gone.Allowed {
		t.Errorf("an app that is gone is for review: %+v", gone)
	}
	if here := by["com.example.Here"]; here.Verdict != Keep || here.Name != "Here" || here.Allowed {
		t.Errorf("installed app (denied): %+v", here)
	}
	if apple := by["com.apple.FaceTime"]; apple.Verdict != Keep {
		t.Errorf("Apple's entries are never offered: %+v", apple)
	}
	if nested := by["com.example.Nested"]; nested.Verdict != Keep || !nested.System {
		t.Errorf("a helper found elsewhere stays: %+v", nested)
	}

	inv := &Inventory{Permissions: perms}
	if n := len(inv.StalePermissions()); n != 2 {
		t.Fatalf("%d stale entries, want 2", n)
	}
	c, err := inv.Resolve("perm-reset:" + old.ID())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.Command, "mac op perm-reset 'SystemPolicyDocumentsFolder' '/Users/me/Library/Application Support/Claude/claude-code/2.0.14/node'") {
		t.Errorf("command %q", c.Command)
	}
	if c.Undo != "" || c.Section != "permissions" {
		t.Errorf("a removed permission cannot be undone: %+v", c)
	}
	report := inv.Report("permissions")
	for _, want := range []string{"7 entries, 2 left over", "perm-reset:SystemPolicyDocumentsFolder:/Users/me", "one of 3 entries named node",
		`lists 2 different "node" binaries that all still exist`} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
}

func TestPermissionsWithoutFullDiskAccess(t *testing.T) {
	fakeTCC(t, "", row("kTCCServiceAccessibility", "/gone/bin/tool", "1", "2", "UNUSED", "0"), errors.New("unable to open database file"))
	pathExists = func(string) bool { return false }
	perms, noAccess := Permissions(&UserPolicy{}, nil)
	if !noAccess {
		t.Fatal("an unreadable user database must be reported")
	}
	if len(perms) != 1 || perms[0].Verdict != Disable {
		t.Fatalf("system entries are still listed: %+v", perms)
	}
	if r := (&Inventory{Permissions: perms, NoTCC: true}).Report("permissions"); !strings.Contains(r, "Full Disk Access") {
		t.Errorf("the report must say how to get the rest:\n%s", r)
	}
}

func TestPermissionRulesKeep(t *testing.T) {
	fakeTCC(t, row("kTCCServiceAccessibility", "/gone/1.2.3/node", "1", "2", "UNUSED", "0"), "", nil)
	pathExists = func(string) bool { return false }
	p := &UserPolicy{}
	if err := p.AddRule("permission-keep", "node"); err != nil {
		t.Fatal(err)
	}
	perms, _ := Permissions(p, nil)
	if perms[0].Verdict != Keep {
		t.Fatalf("the user's rule keeps it: %+v", perms[0])
	}
}

func TestBinaryName(t *testing.T) {
	for path, want := range map[string]string{
		"/Users/me/.nvm/versions/node/v20.11.1/bin/node":               "node 20.11.1",
		"/opt/homebrew/Cellar/python@3.13/3.13.2/bin/python3.13":       "python3.13 3.13.2",
		"/usr/local/bin/tool":                                          "tool",
		"/Applications/Foo.app/Contents/MacOS/helper":                  "helper (Foo)",
		"/Applications/Foo.app/Contents/MacOS/Foo":                     "Foo",
		"/Users/me/Library/Application Support/X/claude-code/2.1/node": "node 2.1",
	} {
		if got := binaryName(path); got != want {
			t.Errorf("binaryName(%q) = %q, want %q", path, got, want)
		}
	}
}

// A real SQLite database stands in for the user's privacy database: tccutil
// refuses the entry (as it does for a path or an uninstalled app), so omega
// deletes that one row, after saving a copy, and leaves the others alone.
func TestResetPermissionDeletesOnlyThatRow(t *testing.T) {
	sqlite, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 is not installed")
	}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	i18n.Init("en")
	dir := t.TempDir()
	userDB, systemDB := filepath.Join(dir, "user.db"), filepath.Join(dir, "system.db")
	schema := "create table access (service text, client text, client_type integer, auth_value integer, indirect_object_identifier text, last_modified integer);"
	seed := func(db, rows string) {
		if out, err := exec.Command(sqlite, db, schema+rows).CombinedOutput(); err != nil {
			t.Fatalf("seeding %s: %v %s", db, err, out)
		}
	}
	seed(userDB,
		"insert into access values ('kTCCServiceSystemPolicyDocumentsFolder','/gone/2.0.14/node',1,2,'UNUSED',1);"+
			"insert into access values ('kTCCServiceSystemPolicyDocumentsFolder','/here/2.0.31/node',1,2,'UNUSED',2);"+
			"insert into access values ('kTCCServiceSystemPolicyDownloadsFolder','/gone/2.0.14/node',1,2,'UNUSED',3);"+
			"insert into access values ('kTCCServiceCamera','com.it''s.quoted',0,2,'UNUSED',4);")
	seed(systemDB, "insert into access values ('kTCCServiceAccessibility','/gone/2.0.14/node',1,2,'UNUSED',5);")

	oldOut, oldDBs := Output, tccDBs
	t.Cleanup(func() { Output, tccDBs = oldOut, oldDBs })
	tccDBs = func() []string { return []string{userDB, systemDB} }
	var ran []string
	Output = func(name string, args ...string) ([]byte, error) {
		ran = append(ran, name)
		switch name {
		case "tccutil":
			return nil, errors.New(`No such bundle identifier`)
		case "killall", "/bin/sh":
			return nil, nil
		case "sqlite3":
			// The system-wide database is read-only for everyone but macOS.
			if len(args) >= 2 && args[0] == systemDB {
				return nil, errors.New("attempt to write a readonly database")
			}
			return exec.Command(sqlite, args...).Output()
		}
		return nil, errors.New("unexpected " + name)
	}
	count := func(db string) string {
		out, _ := exec.Command(sqlite, db, "select count(*) from access").Output()
		return strings.TrimSpace(string(out))
	}

	if err := ResetPermission("kTCCServiceSystemPolicyDocumentsFolder", "/gone/2.0.14/node"); err != nil {
		t.Fatalf("the row of the user's database is removed: %v", err)
	}
	if n := count(userDB); n != "3" {
		t.Fatalf("%s rows left, want 3: only the one entry goes", n)
	}
	if db, _ := permissionListed("SystemPolicyDownloadsFolder", "/gone/2.0.14/node"); db != 0 {
		t.Fatal("the same binary under another permission must stay until it is picked")
	}
	backups, _ := filepath.Glob(filepath.Join(config.StateDir(), "tcc-backup-*.db"))
	if len(backups) != 1 || count(backups[0]) != "4" {
		t.Fatalf("a copy of the database as it was is kept: %v", backups)
	}
	if err := ResetPermission("Camera", "com.it's.quoted"); err != nil || count(userDB) != "2" {
		t.Fatalf("a quote in the client is data, not SQL: %v, %s rows", err, count(userDB))
	}

	ran = nil
	err = ResetPermission("Accessibility", "/gone/2.0.14/node")
	if err == nil || !strings.Contains(err.Error(), "System Settings") {
		t.Fatalf("a system-wide entry cannot be removed and the error says where to do it: %v", err)
	}
	if count(systemDB) != "1" {
		t.Fatal("the system-wide database must not be written")
	}
	if !slices.Contains(ran, "/bin/sh") {
		t.Fatal("the settings pane is opened for the user")
	}
	if err := ResetPermission("Accessibility", "/never/listed"); err != nil {
		t.Fatalf("an entry that is not listed is already gone: %v", err)
	}
}

func TestLeftoversAndApps(t *testing.T) {
	old := Home
	Home = t.TempDir()
	oldOut, oldFound, oldExists, oldRooms, oldARM := Output, bundleFound, pathExists, caskrooms, hostARM
	t.Cleanup(func() {
		Home, Output, bundleFound, pathExists, caskrooms, hostARM = old, oldOut, oldFound, oldExists, oldRooms, oldARM
	})
	lib := filepath.Join(Home, "Library")
	for _, d := range []string{
		"Application Support/com.gone.Editor", "Caches/com.gone.Editor", "Caches/com.here.App.helper", "Caches/com.apple.Safari",
		"Application Support/com.tiny.Thing", "Application Support/Plain Name", "Caches/com.elsewhere.Tool", "Caches/com.here.Other",
	} {
		if err := os.MkdirAll(filepath.Join(lib, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(filepath.Join(lib, "Preferences"), 0o755)
	os.WriteFile(filepath.Join(lib, "Preferences", "com.gone.Editor.plist"), []byte("x"), 0o644)
	Output = func(name string, args ...string) ([]byte, error) {
		switch name {
		case "du":
			p := args[len(args)-1]
			switch {
			case strings.Contains(p, "com.tiny"):
				return []byte("12\t" + p), nil
			case strings.HasSuffix(p, ".app"):
				return []byte("204800\t" + p), nil
			}
			return []byte("10240\t" + p), nil
		case "mdls":
			if strings.Contains(args[len(args)-1], "Old") {
				return []byte("2025-01-02 10:00:00 +0000"), nil
			}
			return []byte("(null)"), nil
		}
		return nil, errors.New("unexpected " + name)
	}
	bundleFound = func(b string) bool { return b == "com.elsewhere.Tool" }
	pathExists = func(string) bool { return false }
	installed := map[string]App{
		"com.here.App": {Name: "App", Bundle: "com.here.App", Path: "/Applications/App.app"},
		"com.old.Old":  {Name: "Old", Bundle: "com.old.Old", Path: "/Applications/Old.app"},
		"com.apple.dt": {Name: "Xcode", Bundle: "com.apple.dt", Path: "/Applications/Xcode.app"},
	}

	left := Leftovers(&UserPolicy{}, installed)
	if len(left) != 2 {
		t.Fatalf("leftovers %+v, want com.gone.Editor and com.here.Other", left)
	}
	if left[0].Bundle != "com.gone.Editor" || len(left[0].Paths) != 3 || left[0].Size != 3*10240*1024 || left[0].Vendor {
		t.Errorf("first leftover %+v", left[0])
	}
	if left[1].Bundle != "com.here.Other" || !left[1].Vendor {
		t.Errorf("a leftover of an installed vendor is flagged as shared: %+v", left[1])
	}
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	c := left[0].TrashChange(now)
	if strings.Contains(c.Command, "rm ") || !strings.Contains(c.Command, filepath.Join(Home, ".Trash", "com.gone.Editor (omega)")) {
		t.Errorf("leftovers go to the Trash, never rm: %q", c.Command)
	}
	if strings.Count(c.Undo, "mv -n") != 3 {
		t.Errorf("undo puts every path back: %q", c.Undo)
	}

	caskrooms = []string{filepath.Join(Home, "Caskroom")}
	meta := filepath.Join(Home, "Caskroom", "old-app", ".metadata", "1.0", "2026", "Casks")
	os.MkdirAll(meta, 0o755)
	os.WriteFile(filepath.Join(meta, "old-app.json"), []byte(`{"token":"old-app","artifacts":[{"app":["Old.app"]}]}`), 0o644)
	hostARM = false
	apps := Apps(&UserPolicy{}, installed, now)
	if len(apps) != 2 {
		t.Fatalf("Apple's apps are left out: %+v", apps)
	}
	var oldApp AppInfo
	for _, a := range apps {
		if a.Bundle == "com.old.Old" {
			oldApp = a
		} else if a.Verdict != Keep {
			t.Errorf("an app with no usage date is not flagged: %+v", a)
		}
	}
	if oldApp.Verdict != Review || oldApp.Why != WhyUnused || oldApp.Cask != "old-app" || oldApp.Size != 204800*1024 {
		t.Errorf("unused cask app: %+v", oldApp)
	}
	if u := oldApp.UninstallChange(now); u.Command != "brew uninstall --cask 'old-app'" || u.Undo != "brew install --cask 'old-app'" {
		t.Errorf("a cask app is removed through Homebrew: %+v", u)
	}
	manual := AppInfo{App: installed["com.here.App"]}
	if u := manual.UninstallChange(now); !strings.HasPrefix(u.Command, "mv -n '/Applications/App.app' ") || !strings.Contains(u.Undo, "'/Applications/App.app'") {
		t.Errorf("other apps go to the Trash and can come back: %+v", u)
	}
	keep := &UserPolicy{}
	keep.AddRule("app-keep", "Old")
	for _, a := range Apps(keep, installed, now) {
		if a.Verdict != Keep {
			t.Errorf("app-keep rule ignored: %+v", a)
		}
	}
}
