package sudo

import (
	"os"
	"path/filepath"
	"testing"
)

// startedBySudo reads the kernel's own description of two processes. The
// runners of the tests have a sudo that asks for no password, so the real
// thing is not exercised there: this checks the reading.
func TestStartedBySudoReadsProc(t *testing.T) {
	root := t.TempDir()
	old := procRoot
	procRoot = root
	defer func() { procRoot = old }()
	proc := func(pid, stat, status string) {
		dir := filepath.Join(root, pid)
		os.MkdirAll(dir, 0o755)
		if stat != "" {
			os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0o644)
		}
		if status != "" {
			os.WriteFile(filepath.Join(dir, "status"), []byte(status), 0o644)
		}
	}
	// The helper as sudo starts it: its parent is sudo, running as root.
	proc("500", "500 (omega) S 400 500 300 0 -1 4194304 100\n", "")
	proc("400", "", "Name:\tsudo\nUmask:\t0022\nState:\tS (sleeping)\nUid:\t1000\t0\t0\t0\nGid:\t1000\t0\t0\t0\n")
	// Run from a shell, with a name made to confuse: "(sudo) x)".
	proc("600", "600 (a) sudo) x) S 450 600 300 0 -1 4194304 100\n", "")
	proc("450", "", "Name:\tbash\nUid:\t1000\t1000\t1000\t1000\n")
	// A program of the user's own that calls itself sudo.
	proc("700", "700 (omega) S 460 700 300 0\n", "")
	proc("460", "", "Name:\tsudo\nUid:\t1000\t1000\t1000\t1000\n")
	// Another program running as root.
	proc("800", "800 (omega) S 470 800 300 0\n", "")
	proc("470", "", "Name:\tcron\nUid:\t0\t0\t0\t0\n")
	// An orphan, adopted by init.
	proc("900", "900 (omega) S 1 900 300 0\n", "")

	for pid, want := range map[int]bool{500: true, 600: false, 700: false, 800: false, 900: false, 12345: false} {
		if got := startedBySudo(pid); got != want {
			t.Errorf("startedBySudo(%d) = %v, want %v", pid, got, want)
		}
	}
}
