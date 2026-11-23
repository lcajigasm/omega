package agent

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// G5: write_file keeps the newest three copies of a file and nothing else
// in the folder is touched.
func TestPruneBackupsKeepsTheNewest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "omega.toml")
	for _, name := range []string{
		"omega.toml",
		"omega.toml.omega-bak-20261028-082341",
		"omega.toml.omega-bak-20261110-130118",
		"omega.toml.omega-bak-20261110-164550",
		"omega.toml.omega-bak-20261116-072645",
		"omega.toml.omega-bak-20261116-072645-2",
		"omega.toml.omega-bak-20261116-072645-10",
		"omega.toml.omega-bak-notes",           // not a stamp: kept
		"other.toml.omega-bak-20200101-000000", // another file's copy: kept
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pruneBackups(path, keepBackups)
	entries, _ := os.ReadDir(dir)
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	sort.Strings(got)
	want := []string{
		"omega.toml",
		"omega.toml.omega-bak-20261116-072645",
		"omega.toml.omega-bak-20261116-072645-10",
		"omega.toml.omega-bak-20261116-072645-2",
		"omega.toml.omega-bak-notes",
		"other.toml.omega-bak-20200101-000000",
	}
	if len(got) != len(want) {
		t.Fatalf("left %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("left %v, want %v", got, want)
		}
	}
}
