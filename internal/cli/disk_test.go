package cli

import (
	"path/filepath"
	"testing"
)

func TestParseMinSize(t *testing.T) {
	for in, want := range map[string]int64{"500": 500 << 20, "500MB": 500 << 20, "2GB": 2 << 30, "1,5g": 3 << 29} {
		if got, err := parseMinSize(in); err != nil || got != want {
			t.Errorf("%s = %d, %v; want %d", in, got, err, want)
		}
	}
	if _, err := parseMinSize("big"); err == nil {
		t.Error("a word was taken for a size")
	}
}

// A folder that is not there is an error, for duplicates as for large.
func TestDiskFolderMustExist(t *testing.T) {
	if _, err := diskFolder([]string{filepath.Join(t.TempDir(), "nope")}, ""); err == nil {
		t.Error("a missing folder was accepted")
	}
	dir := t.TempDir()
	if got, err := diskFolder(nil, dir); err != nil || got != dir {
		t.Errorf("default folder: %q, %v", got, err)
	}
}
