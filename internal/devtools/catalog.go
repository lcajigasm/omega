// Package devtools knows the languages, version managers and command-line
// tools a developer installs on a Mac or a Linux machine: how to detect each
// one, how to install it there, what line activates it in the shell, and
// what in a setup deserves a look (several managers for one language, a binary shadowed on PATH, old
// versions piling up).
package devtools

// Categories, in the order they are shown.
const (
	Lang      = "lang"
	Versions  = "versions"
	Packages  = "packages"
	Shell     = "shell"
	Git       = "git"
	Container = "container"
	AI        = "ai"
)

// Categories is the display order.
var Categories = []string{Lang, Versions, Packages, Shell, Git, Container, AI}

// Tool is one entry of the catalogue.
type Tool struct {
	ID       string
	Name     string
	Category string
	About    string // a few words on what it is
	Bin      string // binary that shows it is installed
	// AltBin is another name the same program is installed under (Debian
	// ships bat as batcat and fd as fdfind).
	AltBin string
	// Check replaces the PATH lookup when a binary's presence proves
	// nothing (macOS ships a /usr/bin/java stub): a command that exits 0.
	Check string
	// Dir is a folder under the home directory that shows it is installed,
	// for tools that are shell functions (nvm, SDKMAN).
	Dir     string
	Version string // arguments that print the version (default --version)
	Install string // "" when omega only detects it
	// Init is the line that activates the tool in the shell's startup file
	// (written here for zsh); Mark is the text that tells it is already there.
	Init, Mark string
	Lang       string // language it manages versions of, for conflict checks
	Config     []string
	Docs       string
}

// Catalog is every tool omega knows, grouped by category, as it is on a Mac
// with zsh. Tools and Find give each entry as it applies to this machine.
var Catalog = []Tool{
	// Languages and runtimes.
	{ID: "node", Name: "Node.js", Category: Lang, About: "JavaScript runtime", Bin: "node", Install: "brew install node",
		Config: []string{"~/.npmrc"}, Docs: "https://nodejs.org/en/about/previous-releases"},
	{ID: "python", Name: "Python", Category: Lang, About: "Python 3", Bin: "python3", Install: "brew install python",
		Config: []string{"~/.config/pip/pip.conf"}, Docs: "https://devguide.python.org/versions/"},
	{ID: "go", Name: "Go", Category: Lang, About: "Go toolchain", Bin: "go", Version: "version", Install: "brew install go",
		Config: []string{"go env (GOPATH, GOBIN, GOFLAGS, GOTOOLCHAIN)"}, Docs: "https://go.dev/doc/devel/release"},
	{ID: "rust", Name: "Rust", Category: Lang, About: "rustc and cargo", Bin: "rustc", Install: "brew install rust",
		Config: []string{"~/.cargo/config.toml"}, Docs: "https://doc.rust-lang.org/cargo/reference/config.html"},
	{ID: "ruby", Name: "Ruby", Category: Lang, About: "macOS ships an old one; use rbenv or mise for a current one", Bin: "ruby",
		Config: []string{"~/.gemrc"}, Docs: "https://www.ruby-lang.org/en/downloads/branches/"},
	{ID: "java", Name: "Java (JDK)", Category: Lang, About: "Temurin JDK", Bin: "java", Check: "/usr/libexec/java_home >/dev/null 2>&1",
		Version: "-version", Install: "brew install --cask temurin", Docs: "https://adoptium.net/support/"},
	{ID: "deno", Name: "Deno", Category: Lang, About: "TypeScript runtime", Bin: "deno", Install: "brew install deno", Docs: "https://docs.deno.com/runtime/"},
	{ID: "bun", Name: "Bun", Category: Lang, About: "JavaScript runtime and bundler", Bin: "bun", Install: "brew install oven-sh/bun/bun",
		Config: []string{"~/.bunfig.toml"}, Docs: "https://bun.sh/docs/runtime/bunfig"},
	{ID: "php", Name: "PHP", Category: Lang, About: "PHP", Bin: "php", Install: "brew install php", Docs: "https://www.php.net/supported-versions.php"},
	{ID: "dotnet", Name: ".NET SDK", Category: Lang, About: "C# and F#", Bin: "dotnet", Install: "brew install --cask dotnet-sdk",
		Docs: "https://dotnet.microsoft.com/platform/support/policy/dotnet-core"},
	{ID: "zig", Name: "Zig", Category: Lang, About: "Zig compiler", Bin: "zig", Version: "version", Install: "brew install zig"},
	{ID: "elixir", Name: "Elixir", Category: Lang, About: "Elixir on Erlang/OTP", Bin: "elixir", Install: "brew install elixir"},
	{ID: "swift", Name: "Swift", Category: Lang, About: "comes with Xcode or its command line tools", Bin: "swift", Check: "xcode-select -p >/dev/null 2>&1",
		Install: "xcode-select --install"},

	// Version managers.
	{ID: "mise", Name: "mise", Category: Versions, About: "one version manager for every language", Bin: "mise", Install: "brew install mise",
		Init: `eval "$(mise activate zsh)"`, Mark: "mise activate", Lang: "node python ruby java go",
		Config: []string{"~/.config/mise/config.toml"}, Docs: "https://mise.jdx.dev/configuration.html"},
	{ID: "asdf", Name: "asdf", Category: Versions, About: "version manager with plugins", Bin: "asdf", Install: "brew install asdf",
		Lang: "node python ruby java go", Config: []string{"~/.asdfrc", "~/.tool-versions"}, Docs: "https://asdf-vm.com/manage/configuration.html"},
	{ID: "fnm", Name: "fnm", Category: Versions, About: "fast Node.js version manager", Bin: "fnm", Install: "brew install fnm",
		Init: `eval "$(fnm env --use-on-cd --shell zsh)"`, Mark: "fnm env", Lang: "node", Docs: "https://github.com/Schniz/fnm/blob/master/docs/configuration.md"},
	{ID: "nvm", Name: "nvm", Category: Versions, About: "Node.js version manager (slows the shell down unless lazy-loaded)", Dir: ".nvm", Lang: "node",
		Config: []string{"~/.nvmrc", "~/.zshrc (NVM_DIR)"}, Docs: "https://github.com/nvm-sh/nvm#readme"},
	{ID: "volta", Name: "Volta", Category: Versions, About: "Node.js toolchain manager", Bin: "volta", Dir: ".volta", Lang: "node", Docs: "https://docs.volta.sh/"},
	{ID: "pyenv", Name: "pyenv", Category: Versions, About: "Python version manager", Bin: "pyenv", Install: "brew install pyenv",
		Init: `eval "$(pyenv init - zsh)"`, Mark: "pyenv init", Lang: "python", Docs: "https://github.com/pyenv/pyenv#readme"},
	{ID: "uv", Name: "uv", Category: Versions, About: "fast Python packages, tools and Python versions", Bin: "uv", Install: "brew install uv",
		Lang: "python", Config: []string{"~/.config/uv/uv.toml"}, Docs: "https://docs.astral.sh/uv/reference/settings/"},
	{ID: "conda", Name: "conda", Category: Versions, About: "Python environments (Anaconda, Miniconda, Miniforge)", Bin: "conda", Lang: "python",
		Config: []string{"~/.condarc"}, Docs: "https://docs.conda.io/projects/conda/en/stable/user-guide/configuration/"},
	{ID: "rbenv", Name: "rbenv", Category: Versions, About: "Ruby version manager", Bin: "rbenv", Install: "brew install rbenv ruby-build",
		Init: `eval "$(rbenv init - zsh)"`, Mark: "rbenv init", Lang: "ruby", Docs: "https://github.com/rbenv/rbenv#readme"},
	{ID: "rustup", Name: "rustup", Category: Versions, About: "Rust toolchain manager", Bin: "rustup", Docs: "https://rust-lang.github.io/rustup/"},
	{ID: "sdkman", Name: "SDKMAN!", Category: Versions, About: "JDKs and JVM tools", Dir: ".sdkman", Lang: "java", Docs: "https://sdkman.io/usage"},

	// Package managers and build tools.
	{ID: "pnpm", Name: "pnpm", Category: Packages, About: "fast, disk-efficient Node.js packages", Bin: "pnpm", Install: "brew install pnpm", Docs: "https://pnpm.io/settings"},
	{ID: "yarn", Name: "Yarn", Category: Packages, About: "Node.js packages", Bin: "yarn", Install: "brew install yarn", Config: []string{"~/.yarnrc.yml"}},
	{ID: "pipx", Name: "pipx", Category: Packages, About: "Python command-line apps, each in its own environment", Bin: "pipx", Install: "brew install pipx",
		Docs: "https://pipx.pypa.io/stable/docs/"},
	{ID: "poetry", Name: "Poetry", Category: Packages, About: "Python dependency management", Bin: "poetry", Install: "brew install poetry",
		Config: []string{"~/Library/Application Support/pypoetry/config.toml"}, Docs: "https://python-poetry.org/docs/configuration/"},
	{ID: "ruff", Name: "Ruff", Category: Packages, About: "Python linter and formatter", Bin: "ruff", Install: "brew install ruff"},
	{ID: "composer", Name: "Composer", Category: Packages, About: "PHP packages", Bin: "composer", Install: "brew install composer"},
	{ID: "cocoapods", Name: "CocoaPods", Category: Packages, About: "iOS and macOS dependencies", Bin: "pod", Install: "brew install cocoapods"},
	{ID: "maven", Name: "Maven", Category: Packages, About: "Java builds", Bin: "mvn", Install: "brew install maven", Config: []string{"~/.m2/settings.xml"}},
	{ID: "gradle", Name: "Gradle", Category: Packages, About: "JVM builds", Bin: "gradle", Install: "brew install gradle",
		Config: []string{"~/.gradle/gradle.properties"}, Docs: "https://docs.gradle.org/current/userguide/build_environment.html"},
	{ID: "cmake", Name: "CMake", Category: Packages, About: "C and C++ builds", Bin: "cmake", Install: "brew install cmake"},
	{ID: "just", Name: "just", Category: Packages, About: "project command runner", Bin: "just", Install: "brew install just"},

	// Shell and terminal.
	{ID: "starship", Name: "Starship", Category: Shell, About: "fast prompt with git and language info", Bin: "starship", Install: "brew install starship",
		Init: `eval "$(starship init zsh)"`, Mark: "starship init", Config: []string{"~/.config/starship.toml"}, Docs: "https://starship.rs/config/"},
	{ID: "fzf", Name: "fzf", Category: Shell, About: "fuzzy finder: ctrl+r history, ctrl+t files", Bin: "fzf", Install: "brew install fzf",
		Init: `source <(fzf --zsh)`, Mark: "fzf --zsh", Docs: "https://junegunn.github.io/fzf/shell-integration/"},
	{ID: "zoxide", Name: "zoxide", Category: Shell, About: "cd that remembers where you go", Bin: "zoxide", Install: "brew install zoxide",
		Init: `eval "$(zoxide init zsh)"`, Mark: "zoxide init"},
	{ID: "direnv", Name: "direnv", Category: Shell, About: "per-project environment variables", Bin: "direnv", Install: "brew install direnv",
		Init: `eval "$(direnv hook zsh)"`, Mark: "direnv hook"},
	{ID: "atuin", Name: "Atuin", Category: Shell, About: "searchable shell history", Bin: "atuin", Install: "brew install atuin",
		Init: `eval "$(atuin init zsh)"`, Mark: "atuin init", Config: []string{"~/.config/atuin/config.toml"}},
	{ID: "eza", Name: "eza", Category: Shell, About: "ls with colours and git status", Bin: "eza", Install: "brew install eza"},
	{ID: "bat", Name: "bat", Category: Shell, About: "cat with syntax highlighting", Bin: "bat", Install: "brew install bat"},
	{ID: "ripgrep", Name: "ripgrep", Category: Shell, About: "fast grep", Bin: "rg", Install: "brew install ripgrep"},
	{ID: "fd", Name: "fd", Category: Shell, About: "fast find", Bin: "fd", Install: "brew install fd"},
	{ID: "jq", Name: "jq", Category: Shell, About: "JSON on the command line", Bin: "jq", Install: "brew install jq"},
	{ID: "yq", Name: "yq", Category: Shell, About: "YAML on the command line", Bin: "yq", Install: "brew install yq"},
	{ID: "tmux", Name: "tmux", Category: Shell, About: "terminal multiplexer", Bin: "tmux", Version: "-V", Install: "brew install tmux", Config: []string{"~/.tmux.conf", "~/.config/tmux/tmux.conf"}},
	{ID: "neovim", Name: "Neovim", Category: Shell, About: "editor", Bin: "nvim", Install: "brew install neovim", Config: []string{"~/.config/nvim/"}},
	{ID: "btop", Name: "btop", Category: Shell, About: "resource monitor", Bin: "btop", Install: "brew install btop"},
	{ID: "tldr", Name: "tldr", Category: Shell, About: "short command examples", Bin: "tldr", Install: "brew install tlrc"},
	{ID: "hyperfine", Name: "hyperfine", Category: Shell, About: "command benchmarks", Bin: "hyperfine", Install: "brew install hyperfine"},
	{ID: "shellcheck", Name: "ShellCheck", Category: Shell, About: "shell script linter", Bin: "shellcheck", Install: "brew install shellcheck"},

	// Git and code review.
	{ID: "git", Name: "git", Category: Git, About: "a current git instead of Apple's", Bin: "git", Install: "brew install git",
		Config: []string{"~/.gitconfig", "~/.config/git/config"}, Docs: "https://git-scm.com/docs/git-config"},
	{ID: "gh", Name: "GitHub CLI", Category: Git, About: "pull requests, issues and Actions", Bin: "gh", Install: "brew install gh", Config: []string{"~/.config/gh/config.yml"}},
	{ID: "glab", Name: "GitLab CLI", Category: Git, About: "merge requests and pipelines", Bin: "glab", Install: "brew install glab"},
	{ID: "git-lfs", Name: "Git LFS", Category: Git, About: "large files in git", Bin: "git-lfs", Install: "brew install git-lfs && git lfs install"},
	{ID: "delta", Name: "delta", Category: Git, About: "readable diffs", Bin: "delta", Install: "brew install git-delta", Docs: "https://dandavison.github.io/delta/configuration.html"},
	{ID: "lazygit", Name: "lazygit", Category: Git, About: "terminal UI for git", Bin: "lazygit", Install: "brew install lazygit"},
	{ID: "pre-commit", Name: "pre-commit", Category: Git, About: "git hook manager", Bin: "pre-commit", Install: "brew install pre-commit"},

	// Containers and cloud.
	{ID: "docker", Name: "Docker Desktop", Category: Container, About: "containers", Bin: "docker", Install: "brew install --cask docker",
		Config: []string{"~/.docker/daemon.json"}, Docs: "https://docs.docker.com/desktop/settings-and-maintenance/settings/"},
	{ID: "orbstack", Name: "OrbStack", Category: Container, About: "light Docker and Linux machines", Bin: "orb", Install: "brew install --cask orbstack"},
	{ID: "colima", Name: "Colima", Category: Container, About: "container runtime without a desktop app", Bin: "colima", Install: "brew install colima docker"},
	{ID: "lazydocker", Name: "lazydocker", Category: Container, About: "terminal UI for Docker", Bin: "lazydocker", Install: "brew install lazydocker"},
	{ID: "kubectl", Name: "kubectl", Category: Container, About: "Kubernetes", Bin: "kubectl", Version: "version --client", Install: "brew install kubernetes-cli", Config: []string{"~/.kube/config"}},
	{ID: "helm", Name: "Helm", Category: Container, About: "Kubernetes packages", Bin: "helm", Version: "version --short", Install: "brew install helm"},
	{ID: "k9s", Name: "k9s", Category: Container, About: "terminal UI for Kubernetes", Bin: "k9s", Version: "version --short", Install: "brew install k9s"},
	{ID: "tofu", Name: "OpenTofu", Category: Container, About: "infrastructure as code", Bin: "tofu", Install: "brew install opentofu"},
	{ID: "terraform", Name: "Terraform", Category: Container, About: "infrastructure as code", Bin: "terraform"},
	{ID: "aws", Name: "AWS CLI", Category: Container, About: "Amazon Web Services", Bin: "aws", Install: "brew install awscli", Config: []string{"~/.aws/config"}},
	{ID: "gcloud", Name: "Google Cloud CLI", Category: Container, About: "Google Cloud", Bin: "gcloud", Install: "brew install --cask gcloud-cli"},
	{ID: "fly", Name: "flyctl", Category: Container, About: "Fly.io", Bin: "fly", Version: "version", Install: "brew install flyctl"},
	{ID: "act", Name: "act", Category: Container, About: "run GitHub Actions locally", Bin: "act", Install: "brew install act"},

	// AI coding assistants.
	{ID: "claude", Name: "Claude Code", Category: AI, About: "Anthropic's coding agent", Bin: "claude", Install: "brew install --cask claude-code",
		Config: []string{"~/.claude/settings.json"}, Docs: "https://docs.claude.com/en/docs/claude-code/settings"},
	{ID: "codex", Name: "Codex CLI", Category: AI, About: "OpenAI's coding agent", Bin: "codex", Install: "brew install --cask codex", Config: []string{"~/.codex/config.toml"}},
	{ID: "gemini", Name: "Gemini CLI", Category: AI, About: "Google's coding agent", Bin: "gemini", Install: "brew install gemini-cli", Config: []string{"~/.gemini/settings.json"}},
	{ID: "opencode", Name: "opencode", Category: AI, About: "open-source coding agent", Bin: "opencode", Install: "brew install opencode", Config: []string{"~/.config/opencode/opencode.json"}},
	{ID: "ollama", Name: "Ollama", Category: AI, About: "local models", Bin: "ollama", Install: "brew install ollama"},
}

// Find returns a catalogue tool by id, binary or name, as it applies to
// this machine.
func Find(s string) (Tool, bool) {
	for _, t := range Tools() {
		if eq(t.ID, s) || eq(t.Bin, s) || eq(t.Name, s) {
			return t, true
		}
	}
	return Tool{}, false
}
