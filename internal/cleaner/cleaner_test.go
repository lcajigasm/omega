package cleaner

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFindArtifacts(t *testing.T) {
	tmp := t.TempDir()
	nm := filepath.Join(tmp, "project", "node_modules")
	if err := os.MkdirAll(nm, 0755); err != nil {
		t.Fatal(err)
	}
	// Make mod time 40 days old
	old := time.Now().AddDate(0, 0, -40)
	_ = os.Chtimes(nm, old, old)

	found := FindArtifacts([]string{tmp}, 30)
	if len(found) != 1 {
		t.Errorf("expected 1 artifact found, got %d", len(found))
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
