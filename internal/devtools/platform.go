package devtools

import (
	"runtime"
	"strings"
)

// Platform is what decides how a tool is installed and activated on a
// machine: the operating system, whether Homebrew is there, the system's
// package manager and the user's shell.
type Platform struct {
	OS    string // runtime.GOOS
	Brew  bool
	Pkg   string // apt, dnf, pacman, zypper, or "" when none of them is there
	Shell string // base name of $SHELL: zsh, bash, fish…
}

// pkgManagers are the Linux package managers omega installs with, by the
// binary that shows each one.
var pkgManagers = []struct{ name, bin string }{
	{"apt", "apt-get"}, {"dnf", "dnf"}, {"pacman", "pacman"}, {"zypper", "zypper"},
}

// Current detects the platform omega runs on. Tests replace it.
var Current = func() Platform {
	p := Platform{OS: runtime.GOOS, Shell: userShell()}
	_, err := lookPath("brew")
	p.Brew = err == nil
	if p.OS == "linux" {
		for _, m := range pkgManagers {
			if _, err := lookPath(m.bin); err == nil {
				p.Pkg = m.name
				break
			}
		}
	}
	return p
}

// pkgInstall is the non-interactive install command of each package manager.
var pkgInstall = map[string]string{
	// With stdin closed, dpkg must not stop to ask about a configuration file.
	"apt":    "sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold ",
	"dnf":    "sudo dnf install -y ",
	"pacman": "sudo pacman -S --needed --noconfirm ",
	"zypper": "sudo zypper --non-interactive install ",
}

// linuxTool is how a catalogue tool differs on Linux.
type linuxTool struct {
	// Package names per package manager; "" where the distribution does not
	// ship the tool, ships a different program under that name, or ships a
	// version too old to be useful.
	Apt, Dnf, Pacman, Zypper string
	// Script is the tool's own installer, as its documentation gives it,
	// used when there is neither Homebrew nor a package.
	Script string
	// Then runs after a package install (git lfs install).
	Then string
	// MacOnly tools are detected when present but never offered.
	MacOnly bool
	// NoBrew is set where the Homebrew formula does not work on Linux.
	NoBrew bool
	// Name, About and Config replace the Mac wording.
	Name, About string
	Config      []string
}

// same is a tool packaged under one name everywhere.
func same(pkg string) linuxTool {
	return linuxTool{Apt: pkg, Dnf: pkg, Pacman: pkg, Zypper: pkg}
}

// linuxTools is the Linux side of the catalogue, by tool id. A tool without
// an entry is installed with Homebrew when its Mac command is a formula, and
// otherwise only detected.
var linuxTools = map[string]linuxTool{
	// Languages and runtimes.
	"node":   {Apt: "nodejs npm", Dnf: "nodejs npm", Pacman: "nodejs npm"},
	"python": {Apt: "python3 python3-pip python3-venv", Dnf: "python3 python3-pip", Pacman: "python python-pip", Zypper: "python3 python3-pip"},
	"go":     {Apt: "golang-go", Dnf: "golang", Pacman: "go", Zypper: "go"},
	"rust": {Script: "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y",
		About: "rustc and cargo, through rustup"},
	"ruby":   {Apt: "ruby-full", Dnf: "ruby ruby-devel", Pacman: "ruby", Zypper: "ruby", About: "Ruby; use rbenv or mise for a version of your choice"},
	"java":   {Apt: "default-jdk", Dnf: "java-latest-openjdk-devel", Pacman: "jdk-openjdk", About: "OpenJDK", NoBrew: true},
	"deno":   {Pacman: "deno", Script: "curl -fsSL https://deno.land/install.sh | sh"},
	"bun":    {Script: "curl -fsSL https://bun.sh/install | bash"},
	"php":    {Apt: "php-cli", Dnf: "php-cli", Pacman: "php"},
	"dotnet": {NoBrew: true},
	"zig":    {Dnf: "zig", Pacman: "zig"},
	"elixir": same("elixir"),
	"swift":  {MacOnly: true, About: "Swift toolchain"},

	// Version managers.
	"mise":  {Pacman: "mise", Script: "curl https://mise.run | sh"},
	"fnm":   {Script: "curl -fsSL https://fnm.vercel.app/install | bash"},
	"pyenv": {Pacman: "pyenv", Script: "curl -fsSL https://pyenv.run | bash"},
	"uv":    {Pacman: "uv", Script: "curl -LsSf https://astral.sh/uv/install.sh | sh"},
	"rbenv": {Apt: "rbenv ruby-build"},

	// Package managers and build tools.
	"pnpm":      {Pacman: "pnpm", Script: "curl -fsSL https://get.pnpm.io/install.sh | sh -"},
	"yarn":      {Pacman: "yarn"},
	"pipx":      {Apt: "pipx", Dnf: "pipx", Pacman: "python-pipx"},
	"poetry":    {Apt: "python3-poetry", Dnf: "poetry", Pacman: "python-poetry", Config: []string{"~/.config/pypoetry/config.toml"}},
	"ruff":      {Dnf: "ruff", Pacman: "ruff", Script: "curl -LsSf https://astral.sh/ruff/install.sh | sh"},
	"composer":  {Apt: "composer", Dnf: "composer", Pacman: "composer"},
	"cocoapods": {MacOnly: true},
	"maven":     same("maven"),
	"gradle":    {Pacman: "gradle"},
	"cmake":     same("cmake"),
	"just":      same("just"),

	// Shell and terminal.
	"starship":   {Pacman: "starship", Script: "curl -sS https://starship.rs/install.sh | sh -s -- -y"},
	"fzf":        same("fzf"),
	"zoxide":     same("zoxide"),
	"direnv":     same("direnv"),
	"atuin":      {Pacman: "atuin", Script: "curl --proto '=https' --tlsv1.2 -LsSf https://setup.atuin.sh | sh"},
	"eza":        {Apt: "eza", Pacman: "eza", Zypper: "eza"},
	"bat":        same("bat"),
	"ripgrep":    same("ripgrep"),
	"fd":         {Apt: "fd-find", Dnf: "fd-find", Pacman: "fd", Zypper: "fd"},
	"jq":         same("jq"),
	"yq":         {Pacman: "go-yq"},
	"tmux":       same("tmux"),
	"neovim":     same("neovim"),
	"btop":       same("btop"),
	"tldr":       same("tealdeer"),
	"hyperfine":  same("hyperfine"),
	"shellcheck": {Apt: "shellcheck", Dnf: "ShellCheck", Pacman: "shellcheck", Zypper: "ShellCheck"},

	// Git and code review.
	"git":        {Apt: "git", Dnf: "git", Pacman: "git", Zypper: "git", About: "version control"},
	"gh":         {Apt: "gh", Dnf: "gh", Pacman: "github-cli", Zypper: "gh"},
	"glab":       {Apt: "glab", Dnf: "glab", Pacman: "glab", Zypper: "glab"},
	"git-lfs":    {Apt: "git-lfs", Dnf: "git-lfs", Pacman: "git-lfs", Zypper: "git-lfs", Then: "git lfs install"},
	"delta":      same("git-delta"),
	"lazygit":    {Pacman: "lazygit"},
	"pre-commit": {Apt: "pre-commit", Dnf: "pre-commit", Pacman: "pre-commit"},

	// Containers and cloud.
	"docker": {Apt: "docker.io", Pacman: "docker", Zypper: "docker", NoBrew: true, Name: "Docker Engine",
		Config: []string{"/etc/docker/daemon.json"}},
	"orbstack": {MacOnly: true},
	"kubectl":  {Pacman: "kubectl"},
	"helm":     {Pacman: "helm"},
	"k9s":      {Pacman: "k9s"},
	"tofu":     {Pacman: "opentofu"},
	"gcloud":   {NoBrew: true},
	"fly":      {Script: "curl -L https://fly.io/install.sh | sh"},
	"act":      {Pacman: "act"},

	// AI coding assistants.
	"claude":   {Script: "curl -fsSL https://claude.ai/install.sh | bash", NoBrew: true},
	"codex":    {NoBrew: true},
	"opencode": {Script: "curl -fsSL https://opencode.ai/install | bash"},
	"ollama":   {Script: "curl -fsSL https://ollama.com/install.sh | sh"},
}

// altBins are the names Debian and Ubuntu give two tools whose usual name
// was taken by another package.
var altBins = map[string]string{"bat": "batcat", "fd": "fdfind"}

func (l linuxTool) pkg(manager string) string {
	switch manager {
	case "apt":
		return l.Apt
	case "dnf":
		return l.Dnf
	case "pacman":
		return l.Pacman
	case "zypper":
		return l.Zypper
	}
	return ""
}

// linuxInstall picks the install command on Linux: Homebrew when it is
// there and has a formula, then the system's package manager, then the
// tool's own installer. "" means omega only detects the tool.
func linuxInstall(t Tool, p Platform) string {
	l := linuxTools[t.ID]
	if l.MacOnly {
		return ""
	}
	formula := strings.HasPrefix(t.Install, "brew install ") && !strings.Contains(t.Install, "--cask")
	if p.Brew && formula && !l.NoBrew {
		return t.Install
	}
	if pkg := l.pkg(p.Pkg); pkg != "" {
		cmd := pkgInstall[p.Pkg] + pkg
		if l.Then != "" {
			cmd += " && " + l.Then
		}
		return cmd
	}
	return l.Script
}

// FromInstaller reports an install command that pipes the tool's official
// install script into a shell. omega's safety policy refuses that pattern
// when a model writes it; the ones in this catalogue are fixed, point at the
// vendor's own address and are shown to the user before they run.
func FromInstaller(command string) bool {
	for _, l := range linuxTools {
		if l.Script != "" && l.Script == command {
			return true
		}
	}
	return false
}

// forShell rewrites a zsh activation line for bash. Other shells keep the
// zsh line, which omega shows but does not write.
func forShell(line, shell string) string {
	if shell != "bash" {
		return line
	}
	if line == "source <(fzf --zsh)" {
		return `eval "$(fzf --bash)"`
	}
	return strings.ReplaceAll(line, "zsh", "bash")
}

// resolve turns a catalogue entry into the tool as it is on a platform:
// its install command, the line that activates it in the user's shell and
// how it is detected.
func resolve(t Tool, p Platform) Tool {
	t.Init, t.Mark = forShell(t.Init, p.Shell), forShell(t.Mark, p.Shell)
	if p.OS != "linux" {
		return t
	}
	l := linuxTools[t.ID]
	t.Install = linuxInstall(t, p)
	// The Mac checks (java_home, xcode-select) mean nothing here: the binary does.
	t.Check = ""
	t.AltBin = altBins[t.ID]
	if l.Name != "" {
		t.Name = l.Name
	}
	if l.About != "" {
		t.About = l.About
	}
	if l.Config != nil {
		t.Config = l.Config
	}
	return t
}

// Tools is the catalogue as it applies to this machine.
func Tools() []Tool {
	p := Current()
	out := make([]Tool, len(Catalog))
	for i, t := range Catalog {
		out[i] = resolve(t, p)
	}
	return out
}

// RCFile is the startup file omega adds activation lines to, as the user
// would write it: ~/.zshrc, or ~/.bashrc when the shell is bash.
func RCFile() string {
	if Current().Shell == "bash" {
		return "~/.bashrc"
	}
	return "~/.zshrc"
}

// CanActivate reports whether omega edits the startup file of the user's
// shell: zsh and bash. For any other shell it shows the lines instead.
func CanActivate() bool {
	s := Current().Shell
	return s == "zsh" || s == "bash"
}
