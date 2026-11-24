package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

// D13 of the fifth review: write_file replaces the file in one step, keeps
// its permissions, leaves no temporary file and writes through a link.
func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "conf")
	if err := os.WriteFile(p, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(p, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "new" {
		t.Fatalf("content %q", b)
	}
	if fi, _ := os.Stat(p); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("a temporary file was left behind: %v", entries)
	}
	if runtime.GOOS == "windows" {
		return
	}
	real := filepath.Join(dir, "dotfiles", "zshrc")
	os.MkdirAll(filepath.Dir(real), 0o755)
	os.WriteFile(real, []byte("a"), 0o644)
	link := filepath.Join(dir, ".zshrc")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(link, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the link was replaced by a file")
	}
	if b, _ := os.ReadFile(real); string(b) != "b" {
		t.Fatalf("the file the link points to was not written: %q", b)
	}
}

// M4: cuts of long text land between characters.
func TestCutsKeepUTF8Whole(t *testing.T) {
	s := strings.Repeat("ñ", 10) // 20 bytes
	for n := 0; n <= 21; n++ {
		if h := headUTF8(s, n); !utf8.ValidString(h) || len(h) > n {
			t.Errorf("headUTF8(%d) = %q", n, h)
		}
		if tl := tailUTF8(s, n); !utf8.ValidString(tl) || len(tl) > n {
			t.Errorf("tailUTF8(%d) = %q", n, tl)
		}
	}
}
