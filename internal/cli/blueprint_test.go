package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/config"
)

// export does not replace a file without a yes or --force.
func TestExportKeepsAFileThatIsThere(t *testing.T) {
	file := filepath.Join(t.TempDir(), "bp.json")
	os.WriteFile(file, []byte("mine"), 0o600)
	var out bytes.Buffer
	err := exportBlueprint(context.Background(), &out, &out, file, false, false, nil)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("without a terminal: %v", err)
	}
	err = exportBlueprint(context.Background(), &out, &out, file, false, true, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(file); string(b) != "mine" {
		t.Errorf("the file was replaced after a no: %q", b)
	}
}

// bootstrap writes the blueprint's routines; a different custom.toml is
// replaced only after a yes at the terminal or with --force, never with
// --yes alone.
func TestBootstrapRoutines(t *testing.T) {
	isolate(t, nil)
	const theirs = "[routines.default]\nsteps = [\"trash\"]\n"
	const mine = "[routines.mine]\nsteps = [\"brew\"]\n"
	file := filepath.Join(t.TempDir(), "bp.json")
	b, _ := json.Marshal(map[string]any{"custom_toml": theirs})
	writeFile(t, file, string(b))
	routines := func() string { got, _ := os.ReadFile(config.CustomPath()); return string(got) }

	// No custom.toml here: --yes writes it.
	var out bytes.Buffer
	if err := (bootstrapRun{yes: true}).run(&out, file); err != nil || routines() != theirs {
		t.Fatalf("new: %v %q\n%s", err, routines(), out.String())
	}
	// The same routines: nothing to do.
	out.Reset()
	if err := (bootstrapRun{yes: true}).run(&out, file); err != nil || !strings.Contains(out.String(), "Nothing to install") {
		t.Errorf("same: %v\n%s", err, out.String())
	}

	// A different one stays with --yes and no terminal.
	writeFile(t, config.CustomPath(), mine)
	out.Reset()
	if err := (bootstrapRun{yes: true}).run(&out, file); err != nil || routines() != mine || !strings.Contains(out.String(), "--force") {
		t.Errorf("--yes replaced it: %v %q\n%s", err, routines(), out.String())
	}
	// At a terminal the answer decides, even with --yes.
	asked := 0
	no := bootstrapRun{yes: true, tty: true, confirm: func(string) bool { asked++; return false }}
	if err := no.run(io.Discard, file); err != nil || asked != 1 || routines() != mine {
		t.Errorf("answered no: %v, asked %d, %q", err, asked, routines())
	}
	// --force replaces it and keeps the old one.
	if err := (bootstrapRun{yes: true, force: true}).run(io.Discard, file); err != nil || routines() != theirs {
		t.Errorf("--force: %v %q", err, routines())
	}
	if got, _ := os.ReadFile(config.CustomPath() + ".bak"); string(got) != mine {
		t.Errorf("backup: %q", got)
	}
}
