package devtools

import "sort"

// windowsTool is how a catalogue tool is installed on Windows: its id in
// winget's community repository and its name in Scoop's main bucket. An
// empty field means that manager does not carry it under a name omega is
// sure of; a tool with neither is detected when present but never offered.
type windowsTool struct {
	Winget, Scoop string
	// Bin replaces the command omega looks for (python3 is python there).
	Bin string
}

// windowsTools is the catalogue on Windows. Tools that only exist for Unix
// shells (asdf, nvm, pyenv, rbenv, SDKMAN, tmux, direnv) and the Mac-only
// ones are left out: they are not offered.
var windowsTools = map[string]windowsTool{
	"node":       {Winget: "OpenJS.NodeJS.LTS", Scoop: "nodejs-lts"},
	"python":     {Scoop: "python", Bin: "python"},
	"go":         {Winget: "GoLang.Go", Scoop: "go"},
	"rust":       {Winget: "Rustlang.Rustup", Scoop: "rustup"},
	"ruby":       {Scoop: "ruby"},
	"java":       {Winget: "EclipseAdoptium.Temurin.21.JDK", Scoop: ""},
	"deno":       {Winget: "DenoLand.Deno", Scoop: "deno"},
	"bun":        {Winget: "Oven-sh.Bun", Scoop: "bun"},
	"php":        {Scoop: "php"},
	"dotnet":     {Scoop: "dotnet-sdk"},
	"zig":        {Winget: "zig.zig", Scoop: "zig"},
	"elixir":     {Scoop: "elixir"},
	"mise":       {Winget: "jdx.mise", Scoop: "mise"},
	"fnm":        {Winget: "Schniz.fnm", Scoop: "fnm"},
	"volta":      {Winget: "Volta.Volta", Scoop: "volta"},
	"uv":         {Winget: "astral-sh.uv", Scoop: "uv"},
	"rustup":     {Winget: "Rustlang.Rustup", Scoop: "rustup"},
	"pnpm":       {Winget: "pnpm.pnpm", Scoop: "pnpm"},
	"yarn":       {Winget: "Yarn.Yarn", Scoop: "yarn"},
	"pipx":       {Scoop: "pipx"},
	"poetry":     {Scoop: "poetry"},
	"ruff":       {Winget: "astral-sh.ruff", Scoop: "ruff"},
	"composer":   {Scoop: "composer"},
	"maven":      {Scoop: "maven"},
	"gradle":     {Scoop: "gradle"},
	"cmake":      {Winget: "Kitware.CMake", Scoop: "cmake"},
	"just":       {Winget: "Casey.Just", Scoop: "just"},
	"starship":   {Winget: "Starship.Starship", Scoop: "starship"},
	"fzf":        {Winget: "junegunn.fzf", Scoop: "fzf"},
	"zoxide":     {Winget: "ajeetdsouza.zoxide", Scoop: "zoxide"},
	"eza":        {Winget: "eza-community.eza", Scoop: "eza"},
	"bat":        {Winget: "sharkdp.bat", Scoop: "bat"},
	"ripgrep":    {Winget: "BurntSushi.ripgrep.MSVC", Scoop: "ripgrep"},
	"fd":         {Winget: "sharkdp.fd", Scoop: "fd"},
	"jq":         {Winget: "jqlang.jq", Scoop: "jq"},
	"yq":         {Winget: "MikeFarah.yq", Scoop: "yq"},
	"neovim":     {Winget: "Neovim.Neovim", Scoop: "neovim"},
	"btop":       {Scoop: "btop"},
	"hyperfine":  {Winget: "sharkdp.hyperfine", Scoop: "hyperfine"},
	"shellcheck": {Winget: "koalaman.shellcheck", Scoop: "shellcheck"},
	"git":        {Winget: "Git.Git", Scoop: "git"},
	"gh":         {Winget: "GitHub.cli", Scoop: "gh"},
	"glab":       {Winget: "GLab.GLab", Scoop: "glab"},
	"git-lfs":    {Winget: "GitHub.GitLFS", Scoop: "git-lfs"},
	"delta":      {Winget: "dandavison.delta", Scoop: "delta"},
	"lazygit":    {Winget: "JesseDuffield.lazygit", Scoop: "lazygit"},
	"pre-commit": {Scoop: "pre-commit"},
	"docker":     {Winget: "Docker.DockerDesktop"},
	"lazydocker": {Winget: "JesseDuffield.Lazydocker", Scoop: "lazydocker"},
	"kubectl":    {Winget: "Kubernetes.kubectl", Scoop: "kubectl"},
	"helm":       {Winget: "Helm.Helm", Scoop: "helm"},
	"k9s":        {Winget: "Derailed.k9s", Scoop: "k9s"},
	"tofu":       {Winget: "OpenTofu.Tofu", Scoop: "opentofu"},
	"terraform":  {Winget: "Hashicorp.Terraform", Scoop: "terraform"},
	"aws":        {Winget: "Amazon.AWSCLI", Scoop: "aws"},
	"gcloud":     {Winget: "Google.CloudSDK"},
	"fly":        {Winget: "Fly-io.flyctl", Scoop: "flyctl"},
	"act":        {Winget: "nektos.act", Scoop: "act"},
	"claude":     {Winget: "Anthropic.ClaudeCode"},
	"ollama":     {Winget: "Ollama.Ollama", Scoop: "ollama"},
	// Present on some Windows machines, never installed by omega there.
	"conda": {}, "tldr": {}, "atuin": {}, "codex": {}, "gemini": {}, "opencode": {},
}

// WingetIDs lists the winget ids of the catalogue, for the check that they
// still exist.
func WingetIDs() []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range windowsTools {
		if w.Winget != "" && !seen[w.Winget] {
			seen[w.Winget] = true
			out = append(out, w.Winget)
		}
	}
	sort.Strings(out)
	return out
}

// windowsInstall picks the install command on Windows: Scoop when it is
// installed and carries the tool (it installs for the user, without
// administrator rights), otherwise winget, which Windows ships. "" means
// omega only detects the tool.
func windowsInstall(t Tool, p Platform) string {
	w, ok := windowsTools[t.ID]
	if !ok {
		return ""
	}
	if p.Scoop && w.Scoop != "" {
		return "scoop install " + w.Scoop
	}
	if p.Winget && w.Winget != "" {
		return "winget install --id " + w.Winget + " --exact --accept-source-agreements --accept-package-agreements --disable-interactivity"
	}
	return ""
}
