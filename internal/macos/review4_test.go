package macos

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// D13: the login items System Events knows are the fallback when sfltool
// does not answer, and an item whose app is gone shows as such.
func TestParseLoginItems(t *testing.T) {
	items := parseLoginItems("Gone App, Dropbox, /Applications/Omega Test Gone.app, /Applications")
	if len(items) != 2 || items[0].Name != "Gone App" || !items[0].Missing || items[1].Missing || items[0].Kind != KindLogin {
		t.Fatalf("%+v", items)
	}
	if parseLoginItems("") != nil || parseLoginItems("odd") != nil {
		t.Fatal("nothing to read")
	}
}

// D13: a dump that failed is not tried again for hours.
func TestFailedBTMIsNotRetried(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	os.MkdirAll(filepath.Dir(btmFailed()), 0o755)
	os.WriteFile(btmFailed(), nil, 0o600)
	start := time.Now()
	if out, ok := CachedBTM(); ok || out != "" || time.Since(start) > time.Second {
		t.Fatalf("retried: %q %v", out, ok)
	}
}

// D14: without a Spotlight date, the app's own files give an estimate.
func TestLastActiveFromTheAppsFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prefs := filepath.Join(home, "Library", "Preferences")
	os.MkdirAll(prefs, 0o755)
	p := filepath.Join(prefs, "com.example.App.plist")
	os.WriteFile(p, nil, 0o644)
	old := time.Now().Add(-200 * 24 * time.Hour)
	os.Chtimes(p, old, old)
	if got := lastActive("com.example.App"); got.Sub(old).Abs() > time.Second {
		t.Fatalf("got %v", got)
	}
	if !lastActive("").IsZero() {
		t.Fatal("no bundle, no estimate")
	}
}
