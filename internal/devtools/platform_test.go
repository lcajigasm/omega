package devtools

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInstallCommandPerPlatform(t *testing.T) {
	mac := Platform{OS: "darwin", Brew: true, Shell: "zsh"}
	ubuntu := Platform{OS: "linux", Pkg: "apt", Shell: "bash"}
	fedora := Platform{OS: "linux", Pkg: "dnf", Shell: "bash"}
	arch := Platform{OS: "linux", Pkg: "pacman", Shell: "zsh"}
	suse := Platform{OS: "linux", Pkg: "zypper", Shell: "bash"}
	linuxbrew := Platform{OS: "linux", Brew: true, Pkg: "apt", Shell: "bash"}
	bare := Platform{OS: "linux", Shell: "bash"}

	for _, c := range []struct {
		id   string
		p    Platform
		want string
	}{
		{"jq", mac, "brew install jq"},
		{"java", mac, "brew install --cask temurin"},
		{"jq", ubuntu, "sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold jq"},
		{"jq", fedora, "sudo dnf install -y jq"},
		{"jq", arch, "sudo pacman -S --needed --noconfirm jq"},
		{"jq", suse, "sudo zypper --non-interactive install jq"},
		// Homebrew on Linux wins for formulae; casks do not exist there.
		{"jq", linuxbrew, "brew install jq"},
		{"java", linuxbrew, "sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold default-jdk"},
		{"docker", linuxbrew, "sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold docker.io"},
		// Package names differ per distribution.
		{"fd", ubuntu, "sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold fd-find"},
		{"fd", arch, "sudo pacman -S --needed --noconfirm fd"},
		{"gh", arch, "sudo pacman -S --needed --noconfirm github-cli"},
		{"shellcheck", fedora, "sudo dnf install -y ShellCheck"},
		{"git-lfs", ubuntu, "sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold git-lfs && git lfs install"},
		// No package: the tool's own installer.
		{"rust", ubuntu, "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y"},
		{"uv", fedora, "curl -LsSf https://astral.sh/uv/install.sh | sh"},
		{"uv", arch, "sudo pacman -S --needed --noconfirm uv"},
		{"mise", bare, "curl https://mise.run | sh"},
		// Nothing that works: omega only detects it.
		{"jq", bare, ""},
		{"orbstack", linuxbrew, ""},
		{"cocoapods", ubuntu, ""},
		{"swift", ubuntu, ""},
		{"lazydocker", ubuntu, ""},
		{"lazydocker", linuxbrew, "brew install lazydocker"},
	} {
		var tool Tool
		for _, raw := range Catalog {
			if raw.ID == c.id {
				tool = resolve(raw, c.p)
			}
		}
		if tool.ID == "" {
			t.Fatalf("no tool %s", c.id)
		}
		if tool.Install != c.want {
			t.Errorf("%s on %+v: %q, want %q", c.id, c.p, tool.Install, c.want)
		}
	}
	// The Mac-only detection commands must not run on Linux.
	for _, raw := range Catalog {
		if got := resolve(raw, ubuntu); got.Check != "" {
			t.Errorf("%s keeps a Mac check on Linux: %q", raw.ID, got.Check)
		}
	}
}

// Every Linux entry belongs to a catalogue tool, and no install command on
// Linux may be a cask or come from a tool that only exists on the Mac.
func TestLinuxTable(t *testing.T) {
	ids := map[string]bool{}
	for _, raw := range Catalog {
		ids[raw.ID] = true
	}
	for id, l := range linuxTools {
		if !ids[id] {
			t.Errorf("linuxTools has %q, which is not in the catalogue", id)
		}
		if l.Script != "" && !strings.Contains(l.Script, "https://") {
			t.Errorf("%s: the installer must come over https: %q", id, l.Script)
		}
		if l.Script != "" && !FromInstaller(l.Script) {
			t.Errorf("%s: FromInstaller must recognise its script", id)
		}
	}
	if FromInstaller("curl https://example.com/x.sh | sh") {
		t.Error("only the catalogue's own scripts count")
	}
	for _, pkg := range []string{"apt", "dnf", "pacman", "zypper", ""} {
		for _, brew := range []bool{true, false} {
			for _, raw := range Catalog {
				cmd := resolve(raw, Platform{OS: "linux", Brew: brew, Pkg: pkg, Shell: "bash"}).Install
				if strings.Contains(cmd, "--cask") || strings.Contains(cmd, "xcode-select") {
					t.Errorf("%s (%s, brew %v): %q does not work on Linux", raw.ID, pkg, brew, cmd)
				}
				if !brew && strings.HasPrefix(cmd, "brew ") {
					t.Errorf("%s (%s): %q needs Homebrew, which is not there", raw.ID, pkg, cmd)
				}
			}
		}
	}
}

func TestCurrentDetectsThePackageManager(t *testing.T) {
	oldLook, oldShell := lookPath, userShell
	t.Cleanup(func() { lookPath, userShell = oldLook, oldShell })
	userShell = func() string { return "bash" }
	have := map[string]bool{"dnf": true, "zypper": true}
	lookPath = func(bin string) (string, error) {
		if have[bin] {
			return "/usr/bin/" + bin, nil
		}
		return "", errors.New("not found")
	}
	p := Current()
	if p.Brew || p.Shell != "bash" {
		t.Fatalf("platform %+v", p)
	}
	if p.OS == "linux" && p.Pkg != "dnf" {
		t.Fatalf("the first package manager found wins: %+v", p)
	}
	if p.OS != "linux" && p.Pkg != "" {
		t.Fatalf("package managers are a Linux matter: %+v", p)
	}
}

func TestBashActivation(t *testing.T) {
	h := withHome(t)
	on(t, Platform{OS: "linux", Pkg: "apt", Shell: "bash"})
	if RCFile() != "~/.bashrc" || !CanActivate() {
		t.Fatalf("bash is edited in ~/.bashrc: %s %v", RCFile(), CanActivate())
	}
	// The login file activates starship; ~/.bashrc has nothing yet.
	os.WriteFile(filepath.Join(h, ".profile"), []byte("eval \"$(starship init bash)\"\n"), 0o644)
	os.WriteFile(filepath.Join(h, ".bashrc"), []byte("alias ll='ls -l'"), 0o644)
	os.WriteFile(filepath.Join(h, ".zshrc"), []byte("eval \"$(zoxide init zsh)\"\n"), 0o644)
	starship, _ := Find("starship")
	zoxide, _ := Find("zoxide")
	fzf, _ := Find("fzf")
	mise, _ := Find("mise")
	for tool, want := range map[*Tool]string{
		&starship: `eval "$(starship init bash)"`, &zoxide: `eval "$(zoxide init bash)"`,
		&fzf: `eval "$(fzf --bash)"`, &mise: `eval "$(mise activate bash)"`,
	} {
		if tool.Init != want {
			t.Errorf("%s: %q, want %q", tool.ID, tool.Init, want)
		}
		if !strings.Contains(tool.Init, tool.Mark) {
			t.Errorf("%s: its own line %q would not count as activated (mark %q)", tool.ID, tool.Init, tool.Mark)
		}
	}
	now := time.Date(2026, 11, 10, 9, 30, 0, 0, time.UTC)
	added, backup, err := AddInit([]Tool{starship, zoxide, fzf}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 2 || added[0].ID != "zoxide" || added[1].ID != "fzf" {
		t.Fatalf("starship is active through ~/.profile and a zsh line does not count for bash: added %+v", added)
	}
	if backup != filepath.Join(h, ".bashrc.omega-bak-20261110-093000") {
		t.Fatalf("backup %q", backup)
	}
	got, _ := os.ReadFile(filepath.Join(h, ".bashrc"))
	want := "alias ll='ls -l'\n\n# Added by omega dev (2026-11-10)\neval \"$(zoxide init bash)\"\neval \"$(fzf --bash)\"\n"
	if string(got) != want {
		t.Fatalf("~/.bashrc is\n%q\nwant\n%q", got, want)
	}
	if zsh, _ := os.ReadFile(filepath.Join(h, ".zshrc")); string(zsh) != "eval \"$(zoxide init zsh)\"\n" {
		t.Fatalf("~/.zshrc must not be touched: %q", zsh)
	}

	on(t, Platform{OS: "linux", Shell: "fish"})
	if CanActivate() {
		t.Fatal("omega edits zsh and bash only")
	}
}

func TestDebianNamesAndLinuxReport(t *testing.T) {
	withHome(t)
	on(t, Platform{OS: "linux", Pkg: "apt", Shell: "bash"})
	oldLook := lookPath
	t.Cleanup(func() { lookPath = oldLook })
	lookPath = func(bin string) (string, error) {
		switch bin {
		case "batcat", "fdfind", "git":
			return "/usr/bin/" + bin, nil
		}
		return "", errors.New("not found")
	}
	bat, _ := Find("bat")
	fd, _ := Find("fd")
	rg, _ := Find("ripgrep")
	if !bat.IsInstalled() || !fd.IsInstalled() || rg.IsInstalled() {
		t.Fatal("Debian installs bat as batcat and fd as fdfind")
	}
	if got := managerOf("/usr/bin/git"); got != "apt" {
		t.Fatalf("a binary in /usr/bin comes from the distribution: %q", got)
	}
	docker, _ := Find("docker")
	if docker.Name != "Docker Engine" {
		t.Fatalf("there is no Docker Desktop to install here: %q", docker.Name)
	}
	mise, _ := Find("mise")
	inv := &Inventory{Platform: Current(), Missing: []Tool{rg, mise}, Installed: []Installed{{Tool: bat, Path: "/usr/bin/batcat", Inactive: false}}}
	r := inv.Report()
	for _, want := range []string{"Linux without Homebrew", "installed with apt", "`sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold ripgrep`", "`curl https://mise.run | sh`",
		"then add `eval \"$(mise activate bash)\"` to ~/.bashrc"} {
		if !strings.Contains(r, want) {
			t.Errorf("report lacks %q:\n%s", want, r)
		}
	}
	if strings.Contains(r, "zshrc") || strings.Contains(r, "brew.sh") {
		t.Errorf("the report must not talk about zsh or ask for Homebrew here:\n%s", r)
	}
}
