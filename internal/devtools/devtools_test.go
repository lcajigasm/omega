package devtools

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseVersion(t *testing.T) {
	for out, want := range map[string]string{
		"v26.13.0\n":                       "26.13.0",
		"go version go1.27.3 darwin/arm64": "1.27.3",
		"git version 2.56.0":               "2.56.0",
		"Python 3.15.2":                    "3.15.2",
		"\x1b[33mWARNING: CocoaPods was not built for this Ruby\x1b[0m\n1.17.0\n":   "1.17.0",
		"eza - A modern, maintained replacement for ls\nv0.23.5 [+git]\n":           "0.23.5",
		"ShellCheck - shell script analysis tool\nversion: 0.11.0\n":                "0.11.0",
		"swift-driver version: 1.168.6 Apple Swift version 6.4.2 (swiftlang-6.4.2)": "6.4.2",
		"openjdk version \"21.0.4\" 2024-07-16":                                     "21.0.4",
		"tmux 3.5a":                                                                 "3.5a",
		"":                                                                          "",
	} {
		if got := parseVersion(out); got != want {
			t.Errorf("parseVersion(%q) = %q, want %q", out, got, want)
		}
	}
}

func withHome(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	// macOS temp dirs are reached through a symlink; resolve it so paths compare.
	if r, err := filepath.EvalSymlinks(h); err == nil {
		h = r
	}
	old := home
	home = func() string { return h }
	t.Setenv("ZDOTDIR", "")
	t.Cleanup(func() { home = old })
	return h
}

func TestManagerOf(t *testing.T) {
	h := withHome(t)
	for path, want := range map[string]string{
		"/opt/homebrew/bin/node":                                  "Homebrew",
		h + "/.nvm/versions/node/v20.1.0/bin/node":                "nvm",
		h + "/.local/share/mise/installs/node/22/bin/node":        "mise",
		h + "/.cargo/bin/rg":                                      "cargo",
		h + "/.pyenv/shims/python3":                               "pyenv",
		h + "/go/bin/gopls":                                       "go install",
		"/usr/bin/git":                                            "macOS",
		"/Applications/Docker.app/Contents/Resources/bin/kubectl": "app bundle",
		"/somewhere/else/tool":                                    "",
	} {
		if got := managerOf(path); got != want {
			t.Errorf("managerOf(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestActivationAndAddInit(t *testing.T) {
	h := withHome(t)
	rc := filepath.Join(h, ".zshrc")
	os.WriteFile(rc, []byte("export EDITOR=vim\nplugins=(git zoxide)\neval \"$(starship init zsh)\""), 0o644)
	starship, _ := Find("starship")
	zoxide, _ := Find("zoxide")
	fzf, _ := Find("fzf")
	jq, _ := Find("jq")
	text := rcText()
	if !activated(starship, text) || !activated(zoxide, text) || !activated(jq, text) {
		t.Fatal("starship has its line, zoxide an Oh My Zsh plugin and jq needs nothing")
	}
	if activated(fzf, text) {
		t.Fatal("fzf has no line yet")
	}
	now := time.Date(2026, 11, 10, 9, 30, 0, 0, time.UTC)
	added, backup, err := AddInit([]Tool{starship, zoxide, fzf, jq}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 || added[0].ID != "fzf" {
		t.Fatalf("only fzf was missing, added %+v", added)
	}
	old, err := os.ReadFile(backup)
	if err != nil || !strings.HasSuffix(string(old), `eval "$(starship init zsh)"`) {
		t.Fatalf("the copy must hold the file as it was: %q %v", old, err)
	}
	got, _ := os.ReadFile(rc)
	want := "export EDITOR=vim\nplugins=(git zoxide)\neval \"$(starship init zsh)\"\n\n# Added by omega dev (2026-11-10)\nsource <(fzf --zsh)\n"
	if string(got) != want {
		t.Fatalf("~/.zshrc is\n%q\nwant\n%q", got, want)
	}
	if again, _, _ := AddInit([]Tool{fzf}, now); len(again) != 0 {
		t.Fatal("a second run must add nothing")
	}
}

func TestShadows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell PATH lookups and the executable bit are Unix notions")
	}
	h := withHome(t)
	mk := func(dir, name, content string) string {
		os.MkdirAll(filepath.Join(h, dir), 0o755)
		p := filepath.Join(h, dir, name)
		os.WriteFile(p, []byte(content), 0o755)
		return p
	}
	a := mk("nvm/bin", "node", "a")
	b := mk("brew/bin", "node", "b")
	mk("brew/bin", "go", "only one")
	os.Symlink(a, filepath.Join(h, "brew/bin", "npm"))
	mk("nvm/bin", "npm", "same file reached twice is not a shadow")
	os.Remove(filepath.Join(h, "nvm/bin", "npm"))
	os.Symlink(a, filepath.Join(h, "nvm/bin", "npm"))
	got := shadows([]string{filepath.Join(h, "nvm/bin"), filepath.Join(h, "brew/bin"), filepath.Join(h, "nvm/bin")})
	if len(got) != 1 || got[0].Bin != "node" || len(got[0].Paths) != 2 || got[0].Paths[0] != a || got[0].Paths[1] != b {
		t.Fatalf("shadows %+v, want node with the nvm copy first", got)
	}
}

func TestConflictsCountOnlyManagersInUse(t *testing.T) {
	h := withHome(t)
	nvm, _ := Find("nvm")
	fnm, _ := Find("fnm")
	mise, _ := Find("mise")
	uv, _ := Find("uv")
	pyenv, _ := Find("pyenv")
	inst := func(ts ...Tool) []Installed {
		var out []Installed
		for _, t := range ts {
			out = append(out, Installed{Tool: t})
		}
		return out
	}
	if c := conflicts(inst(nvm, mise, uv, pyenv)); len(c) != 0 {
		t.Fatalf("mise and uv hold no versions yet: %+v", c)
	}
	os.MkdirAll(filepath.Join(h, ".local/share/mise/installs/node/22.1.0"), 0o755)
	os.MkdirAll(filepath.Join(h, ".local/share/uv/python/cpython-3.13"), 0o755)
	c := conflicts(inst(nvm, fnm, mise, uv, pyenv))
	if len(c) != 2 || c[0].Lang != "node" || len(c[0].Managers) != 3 || c[1].Lang != "python" || len(c[1].Managers) != 2 {
		t.Fatalf("conflicts %+v", c)
	}
}

func TestReportAndCatalog(t *testing.T) {
	seen := map[string]bool{}
	for _, tool := range Catalog {
		if seen[tool.ID] {
			t.Errorf("duplicate tool id %s", tool.ID)
		}
		seen[tool.ID] = true
		if tool.Bin == "" && tool.Dir == "" && tool.Check == "" {
			t.Errorf("%s cannot be detected", tool.ID)
		}
		if (tool.Init == "") != (tool.Mark == "") {
			t.Errorf("%s: Init and Mark go together", tool.ID)
		}
		if tool.Init != "" && !strings.Contains(tool.Init, tool.Mark) {
			t.Errorf("%s: its own line %q would not count as activated (mark %q)", tool.ID, tool.Init, tool.Mark)
		}
		ok := false
		for _, c := range Categories {
			ok = ok || c == tool.Category
		}
		if !ok {
			t.Errorf("%s: unknown category %q", tool.ID, tool.Category)
		}
	}
	fzf, _ := Find("FZF")
	node, _ := Find("node")
	mise, _ := Find("mise")
	inv := &Inventory{
		Brew:      true,
		Installed: []Installed{{Tool: node, Path: "/opt/homebrew/bin/node", Version: "24.1.0", Manager: "Homebrew"}, {Tool: fzf, Path: "/opt/homebrew/bin/fzf", Version: "0.60", Inactive: true}},
		Missing:   []Tool{mise},
		PathDup:   []string{"/opt/homebrew/bin"},
		Shadows:   []Shadow{{Bin: "node", Paths: []string{"/a/node", "/b/node"}}},
		Conflicts: []Conflict{{Lang: "node", Managers: []string{"nvm", "fnm"}}},
		Stores:    []Store{{Name: "Node.js (nvm)", Dir: "/x", Versions: []string{"v18", "v20", "v22"}, Size: 3 << 30}},
		Startup:   900 * time.Millisecond,
	}
	if n := inv.Findings(); n != 6 {
		t.Errorf("findings %d, want 6", n)
	}
	r := inv.Report()
	for _, want := range []string{"Node.js [lang] 24.1.0 at /opt/homebrew/bin/node (installed by Homebrew)", "NOT ACTIVATED", "`brew install mise`",
		"then add `eval \"$(mise activate zsh)\"`", "several managers: nvm, fnm", "/a/node > /b/node", "900 ms — slow", "3 versions, 3.0 GB", "never with rm"} {
		if !strings.Contains(r, want) {
			t.Errorf("report lacks %q:\n%s", want, r)
		}
	}
}
