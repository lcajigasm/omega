package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// M9 of the fifth review: the web cache drops what is older than a day,
// and fetch_url reads text only.
func TestWebCacheIsPruned(t *testing.T) {
	dir := t.TempDir()
	old, fresh := filepath.Join(dir, "old"), filepath.Join(dir, "fresh")
	os.WriteFile(old, []byte("x"), 0o600)
	os.WriteFile(fresh, []byte("x"), 0o600)
	past := time.Now().Add(-48 * time.Hour)
	os.Chtimes(old, past, past)
	pruneWebCache(dir, time.Now())
	if _, err := os.Stat(old); err == nil {
		t.Error("an entry older than a day was kept")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("a fresh entry was deleted")
	}
}

func TestFetchReadsTextOnly(t *testing.T) {
	for ctype, want := range map[string]bool{
		"text/html; charset=utf-8": true,
		"text/plain":               true,
		"application/json":         true,
		"application/xhtml+xml":    true,
		"application/toml":         true,
		"application/pdf":          false,
		"image/png":                false,
		"application/octet-stream": false,
		"application/zip":          false,
	} {
		if got := textual(ctype, "body"); got != want {
			t.Errorf("textual(%q) = %v, want %v", ctype, got, want)
		}
	}
	if textual("", "%PDF-1.7\n\x00\x01binary") {
		t.Error("a PDF without a content type was taken for text")
	}
	if !textual("", "<html><body>hi</body></html>") {
		t.Error("a page without a content type is text")
	}
}
