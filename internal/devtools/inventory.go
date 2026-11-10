package devtools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func eq(a, b string) bool { return a != "" && strings.EqualFold(a, b) }

// Seams the tests replace.
var (
	home = func() string {
		h, _ := os.UserHomeDir()
		return h
	}
	lookPath = exec.LookPath
	// run executes a program directly (no shell) and returns its combined output.
	run = func(timeout time.Duration, name string, args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
		return string(out), err
	}
	userShell = func() string { return filepath.Base(os.Getenv("SHELL")) }
)

// Installed is a catalogue tool found on this machine.
type Installed struct {
	Tool
	Path    string // where the binary that runs lives ("" for shell functions)
	Version string
	Manager string // what installed it, worked out from the path
	// Inactive is set when the tool needs a line in the shell startup file
	// and none was found: it is installed but does nothing.
	Inactive bool
}

// Shadow is a command found more than once on PATH in a way that is
// probably not intended.
type Shadow struct {
	Bin   string
	Paths []string // in PATH order: the first one runs
}

// Conflict is a language with several version managers installing it.
type Conflict struct {
	Lang     string
	Managers []string
}

// Store is a folder where a manager keeps one copy per installed version.
type Store struct {
	Name     string
	Dir      string
	Versions []string
	Size     int64 // bytes
}

// Inventory is the state of the development setup.
type Inventory struct {
	Installed []Installed
	Missing   []Tool   // catalogue tools that are not installed and omega can install
	Path      []string // PATH of a login interactive shell
	PathDup   []string // entries listed more than once
	PathGone  []string // entries that do not exist
	Shadows   []Shadow
	Conflicts []Conflict
	Stores    []Store
	// Startup is how long an interactive shell takes to start (0 when not measured).
	Startup time.Duration
	Brew    bool
	// Platform is what the install commands and shell lines were chosen for.
	Platform Platform
}

// IsInstalled reports whether the tool is on this machine.
func (t Tool) IsInstalled() bool {
	if t.Check != "" {
		_, err := run(5*time.Second, "/bin/sh", "-c", t.Check)
		return err == nil
	}
	if t.binPath() != "" {
		return true
	}
	if t.Dir != "" {
		fi, err := os.Stat(filepath.Join(home(), t.Dir))
		return err == nil && fi.IsDir()
	}
	return false
}

// binPath is where the tool's binary is, under its usual or its other name.
func (t Tool) binPath() string {
	for _, bin := range []string{t.Bin, t.AltBin} {
		if bin == "" {
			continue
		}
		if p, err := lookPath(bin); err == nil {
			return p
		}
	}
	return ""
}

var (
	versionNumber = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?[\w.+-]*`)
	versionWord   = regexp.MustCompile(`(?i)\bversion[:= ]+v?(\d+\.\d+(?:\.\d+)?[\w.+-]*)`)
	ansiCode      = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
)

// parseVersion pulls the version number out of what a tool printed. Tools
// print banners, warnings and several numbers ("swift-driver version: 1.1
// Apple Swift version 6.2"), so the number after the last "version" on the
// first line that has one wins, then the first number anywhere.
func parseVersion(out string) string {
	var lines []string
	for _, line := range strings.Split(ansiCode.ReplaceAllString(out, ""), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.Contains(strings.ToLower(line), "warning") {
			lines = append(lines, line)
		}
	}
	for _, line := range lines {
		if m := versionWord.FindAllStringSubmatch(line, -1); len(m) > 0 {
			return strings.TrimRight(m[len(m)-1][1], ".,")
		}
	}
	for _, line := range lines {
		if v := versionNumber.FindString(line); v != "" {
			return strings.TrimRight(v, ".,")
		}
	}
	return ""
}

// managerOf says what installed a binary, from its resolved path.
func managerOf(path string) string {
	real := path
	if r, err := filepath.EvalSymlinks(path); err == nil {
		real = r
	}
	// Windows paths are compared with forward slashes, like the rest.
	real, path = filepath.ToSlash(real), filepath.ToSlash(path)
	h := filepath.ToSlash(home())
	in := func(parts ...string) bool {
		for _, p := range parts {
			if strings.Contains(real, p) || strings.Contains(path, p) {
				return true
			}
		}
		return false
	}
	switch {
	case in("/scoop/shims/", "/scoop/apps/"):
		return "Scoop"
	case in("/WinGet/Links/", "/WinGet/Packages/"):
		return "winget"
	case in("/chocolatey/"):
		return "Chocolatey"
	case in("/Cellar/", "/Caskroom/", "/opt/homebrew/", "/usr/local/Homebrew/", "/.linuxbrew/"):
		return "Homebrew"
	case strings.HasPrefix(real, "/snap/") || strings.HasPrefix(path, "/snap/"):
		return "snap"
	case in("/mise/installs/", "/mise/shims/"):
		return "mise"
	case in(h + "/.asdf/"):
		return "asdf"
	case in(h + "/.nvm/"):
		return "nvm"
	case in("/fnm_multishells/", "/fnm/"):
		return "fnm"
	case in(h + "/.volta/"):
		return "Volta"
	case in(h + "/.pyenv/"):
		return "pyenv"
	case in(h + "/.rbenv/"):
		return "rbenv"
	case in(h + "/.sdkman/"):
		return "SDKMAN!"
	case in("/miniconda", "/anaconda", "/miniforge", "/mambaforge"):
		return "conda"
	case in(h + "/.cargo/bin/"):
		return "cargo"
	case in(h + "/.bun/"):
		return "bun"
	case in(h + "/.deno/"):
		return "deno"
	case in(h + "/go/bin/"):
		return "go install"
	case in(h+"/Library/pnpm/", "/pnpm/global/"):
		return "pnpm"
	case in("/lib/node_modules/", h+"/.npm-global/"):
		return "npm"
	case in(h + "/.local/"):
		return "~/.local"
	case in(".app/Contents/"):
		return "app bundle"
	case strings.HasPrefix(real, "/usr/bin/") || strings.HasPrefix(real, "/bin/") || strings.HasPrefix(real, "/usr/sbin/") || in("/Library/Developer/CommandLineTools/", "/Xcode.app/"):
		return systemName()
	case strings.HasPrefix(real, "/usr/local/"):
		return "/usr/local"
	}
	return ""
}

// systemName is what a binary under /usr/bin came from: macOS itself, or
// the distribution's package manager.
func systemName() string {
	p := Current()
	switch {
	case p.OS == "darwin":
		return "macOS"
	case p.Pkg != "":
		return p.Pkg
	}
	return "system"
}

// rcFiles are the startup files of the user's shell that omega reads to see
// what is activated. The first one is where it adds lines: ~/.zshrc, or
// ~/.bashrc when the shell is bash.
func rcFiles() []string {
	if Current().Shell == "bash" {
		h := home()
		return []string{filepath.Join(h, ".bashrc"), filepath.Join(h, ".bash_profile"), filepath.Join(h, ".bash_login"), filepath.Join(h, ".profile")}
	}
	dir := os.Getenv("ZDOTDIR")
	if dir == "" {
		dir = home()
	}
	return []string{filepath.Join(dir, ".zshrc"), filepath.Join(dir, ".zprofile"), filepath.Join(home(), ".zshenv")}
}

func rcText() string {
	var b strings.Builder
	for _, f := range rcFiles() {
		if data, err := os.ReadFile(f); err == nil {
			b.Write(data)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

var omzPlugins = regexp.MustCompile(`(?s)plugins=\(([^)]*)\)`)

// activated reports whether the shell startup files set the tool up, either
// with its own line or through an Oh My Zsh plugin of the same name.
func activated(t Tool, rc string) bool {
	if t.Init == "" || strings.Contains(rc, t.Mark) {
		return true
	}
	for _, m := range omzPlugins.FindAllStringSubmatch(rc, -1) {
		for _, p := range strings.Fields(m[1]) {
			if p == t.ID {
				return true
			}
		}
	}
	return false
}

// freshPath is the PATH a terminal app starts its shell with.
func freshPath() string {
	if Current().OS == "linux" {
		return "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}
	return "/usr/bin:/bin:/usr/sbin:/sbin"
}

// shellPath runs an interactive login shell (zsh or bash) from a clean PATH
// and returns the one it builds. Tests replace it.
var shellPath = func(shell string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-lic", `printf '%s\n' "__OMEGA_PATH__${PATH}"`)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "PATH=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "PATH="+freshPath())
	out, _ := cmd.CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		if _, p, ok := strings.Cut(line, "__OMEGA_PATH__"); ok && p != "" {
			return strings.TrimSpace(p), true
		}
	}
	return "", false
}

// loginPath is PATH as a new terminal has it. omega's own PATH will not do:
// it inherits whatever started omega and has extra entries added, which
// would show up as duplicates and missing folders that are not the user's.
func loginPath() []string {
	raw := os.Getenv("PATH")
	if CanActivate() {
		if p, ok := shellPath(Current().Shell); ok {
			raw = p
		}
	}
	return filepath.SplitList(raw)
}

// shadowBins are the commands where running the wrong copy hurts.
var shadowBins = []string{"node", "npm", "python3", "pip3", "ruby", "gem", "go", "java", "git", "cargo", "rustc", "pnpm", "yarn", "bun", "deno", "php"}

func isSystem(p string) bool {
	return strings.HasPrefix(p, "/usr/bin/") || strings.HasPrefix(p, "/bin/")
}

// shadows finds commands with several different copies on PATH. Homebrew's
// copy ahead of the one macOS ships is the normal case and is not reported;
// the system copy winning over one the user installed is, and so are two
// copies the user installed.
func shadows(path []string) []Shadow {
	var out []Shadow
	for _, bin := range shadowBins {
		var found, reals []string
		seen := map[string]bool{}
		for _, dir := range path {
			p := filepath.Join(dir, bin)
			fi, err := os.Stat(p)
			if err != nil || fi.IsDir() || fi.Mode()&0o111 == 0 {
				continue
			}
			real, err := filepath.EvalSymlinks(p)
			if err != nil {
				real = p
			}
			if seen[real] {
				continue
			}
			seen[real] = true
			found = append(found, p)
			reals = append(reals, real)
		}
		if len(found) < 2 {
			continue
		}
		own := 0
		for _, r := range reals {
			if !isSystem(r) {
				own++
			}
		}
		if own >= 2 || (isSystem(reals[0]) && own >= 1) {
			out = append(out, Shadow{Bin: bin, Paths: found})
		}
	}
	return out
}

// langDirs are the folder names asdf and mise use for each language.
var langDirs = map[string][]string{"node": {"node", "nodejs"}, "python": {"python"}, "ruby": {"ruby"}, "java": {"java"}, "go": {"go", "golang"}}

func hasEntries(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}

// manages reports whether an installed manager actually holds versions of lang.
func manages(t Tool, lang string) bool {
	if !strings.Contains(" "+t.Lang+" ", " "+lang+" ") {
		return false
	}
	h := home()
	var roots []string
	switch t.ID {
	case "mise":
		roots = []string{filepath.Join(h, ".local/share/mise/installs")}
	case "asdf":
		roots = []string{filepath.Join(h, ".asdf/installs")}
	case "uv":
		return hasEntries(filepath.Join(h, ".local/share/uv/python"))
	default:
		return true
	}
	for _, root := range roots {
		for _, name := range langDirs[lang] {
			if hasEntries(filepath.Join(root, name)) {
				return true
			}
		}
	}
	return false
}

func conflicts(installed []Installed) []Conflict {
	var out []Conflict
	for _, lang := range []string{"node", "python", "ruby", "java"} {
		var who []string
		for _, t := range installed {
			if manages(t.Tool, lang) {
				who = append(who, t.Name)
			}
		}
		if len(who) >= 2 {
			out = append(out, Conflict{Lang: lang, Managers: who})
		}
	}
	return out
}

func versionDirs(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var out []string
	for _, e := range entries {
		// mise and rustup link aliases ("20" → "20.11.1"): count real folders.
		if e.Type()&os.ModeSymlink != 0 || !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

func dirSize(dir string) int64 {
	out, _ := run(60*time.Second, "du", "-sk", dir)
	kb, _ := strconv.ParseInt(strings.Fields(out + " 0")[0], 10, 64)
	return kb * 1024
}

// stores lists the folders that hold one copy per version of a language.
func stores() []Store {
	h := home()
	fixed := []struct{ name, dir string }{
		{"Node.js (nvm)", ".nvm/versions/node"},
		{"Node.js (fnm)", "Library/Application Support/fnm/node-versions"},
		{"Node.js (fnm)", ".local/share/fnm/node-versions"},
		{"Node.js (Volta)", ".volta/tools/image/node"},
		{"Python (pyenv)", ".pyenv/versions"},
		{"Python (uv)", ".local/share/uv/python"},
		{"Ruby (rbenv)", ".rbenv/versions"},
		{"Rust toolchains (rustup)", ".rustup/toolchains"},
		{"Java (SDKMAN!)", ".sdkman/candidates/java"},
	}
	var out []Store
	for _, f := range fixed {
		out = append(out, Store{Name: f.name, Dir: filepath.Join(h, f.dir)})
	}
	out = append(out, Store{Name: "Java (system)", Dir: "/Library/Java/JavaVirtualMachines"})
	// Go keeps downloaded toolchains next to the golang.org/x modules.
	if tc := versionDirs(filepath.Join(h, "go/pkg/mod/golang.org")); len(tc) > 0 {
		st := Store{Name: "Go toolchains", Dir: filepath.Join(h, "go/pkg/mod/golang.org")}
		for _, name := range tc {
			if v, ok := strings.CutPrefix(name, "toolchain@v0.0.1-"); ok {
				st.Versions = append(st.Versions, v)
			}
		}
		if len(st.Versions) > 0 {
			st.Size = -1 // measured below from the toolchain folders only
			out = append(out, st)
		}
	}
	for manager, root := range map[string]string{"mise": ".local/share/mise/installs", "asdf": ".asdf/installs"} {
		langs, _ := os.ReadDir(filepath.Join(h, root))
		for _, l := range langs {
			if l.IsDir() {
				out = append(out, Store{Name: l.Name() + " (" + manager + ")", Dir: filepath.Join(h, root, l.Name())})
			}
		}
	}
	var kept []Store
	for _, s := range out {
		if s.Size == -1 {
			kept = append(kept, s)
		} else if s.Versions = versionDirs(s.Dir); len(s.Versions) > 0 {
			kept = append(kept, s)
		}
	}
	var wg sync.WaitGroup
	for i := range kept {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if kept[i].Size == -1 {
				kept[i].Size = 0
				for _, v := range kept[i].Versions {
					kept[i].Size += dirSize(filepath.Join(kept[i].Dir, "toolchain@v0.0.1-"+v))
				}
				return
			}
			kept[i].Size = dirSize(kept[i].Dir)
		}(i)
	}
	wg.Wait()
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Size > kept[j].Size })
	return kept
}

// startupTime is the faster of two interactive starts of the user's shell
// (zsh or bash).
func startupTime() time.Duration {
	if !CanActivate() {
		return 0
	}
	shell := Current().Shell
	best := time.Duration(0)
	for i := 0; i < 2; i++ {
		start := time.Now()
		if _, err := run(20*time.Second, shell, "-i", "-c", "exit"); err != nil {
			return 0
		}
		if d := time.Since(start); best == 0 || d < best {
			best = d
		}
	}
	return best
}

// Detect lists which catalogue tools are installed, without versions or the
// slower checks. It is enough to offer what is missing.
func Detect() (installed []Tool, missing []Tool) {
	for _, t := range Tools() {
		if t.IsInstalled() {
			installed = append(installed, t)
		} else if t.Install != "" {
			missing = append(missing, t)
		}
	}
	return
}

// Load inspects the development setup. deep also measures the shell's
// startup time and the size of the version stores.
func Load(deep bool) *Inventory {
	inv := &Inventory{Platform: Current()}
	inv.Brew = inv.Platform.Brew
	have, missing := Detect()
	inv.Missing = missing
	rc := rcText()
	inv.Installed = make([]Installed, len(have))
	var wg sync.WaitGroup
	for i, t := range have {
		it := Installed{Tool: t, Inactive: !activated(t, rc)}
		if p := t.binPath(); p != "" {
			it.Path = p
			it.Manager = managerOf(p)
		}
		inv.Installed[i] = it
		if it.Path == "" {
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := strings.Fields(inv.Installed[i].Tool.Version)
			if len(args) == 0 {
				args = []string{"--version"}
			}
			out, _ := run(6*time.Second, inv.Installed[i].Path, args...)
			inv.Installed[i].Version = parseVersion(out)
		}(i)
	}
	if deep {
		wg.Add(2)
		go func() { defer wg.Done(); inv.Stores = stores() }()
		go func() { defer wg.Done(); inv.Startup = startupTime() }()
	}
	inv.Path = loginPath()
	seen := map[string]bool{}
	for _, dir := range inv.Path {
		switch {
		case dir == "":
		case seen[dir]:
			inv.PathDup = append(inv.PathDup, dir)
		default:
			// macOS itself lists cryptex folders that only exist on some systems.
			if _, err := os.Stat(dir); err != nil && !strings.HasPrefix(dir, "/var/run/com.apple.security.cryptexd/") {
				inv.PathGone = append(inv.PathGone, dir)
			}
		}
		seen[dir] = true
	}
	inv.Shadows = shadows(inv.Path)
	wg.Wait()
	inv.Conflicts = conflicts(inv.Installed)
	return inv
}

// Inactive are the installed tools that still need their line in the shell's
// startup file.
func (inv *Inventory) Inactive() []Installed {
	var out []Installed
	for _, t := range inv.Installed {
		if t.Inactive {
			out = append(out, t)
		}
	}
	return out
}

// SlowStartup is when a shell start is worth looking into.
const SlowStartup = 500 * time.Millisecond

// Findings counts what deserves attention.
func (inv *Inventory) Findings() int {
	n := len(inv.PathDup) + len(inv.PathGone) + len(inv.Shadows) + len(inv.Conflicts) + len(inv.Inactive())
	if inv.Startup > SlowStartup {
		n++
	}
	for _, s := range inv.Stores {
		if len(s.Versions) >= 3 {
			n++
		}
	}
	return n
}

// HumanSize formats bytes.
func HumanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
}

func tilde(p string) string {
	if h := home(); h != "" && strings.HasPrefix(p, h) {
		return "~" + strings.TrimPrefix(p, h)
	}
	return p
}

// Report is the inventory as text for the agent.
func (inv *Inventory) Report() string {
	var b strings.Builder
	rc := RCFile()
	b.WriteString("## Installed development tools\n")
	for _, cat := range Categories {
		for _, t := range inv.Installed {
			if t.Category != cat {
				continue
			}
			fmt.Fprintf(&b, "- %s [%s] %s", t.Name, cat, orDash(t.Version))
			if t.Path != "" {
				fmt.Fprintf(&b, " at %s", tilde(t.Path))
			}
			if t.Manager != "" {
				fmt.Fprintf(&b, " (installed by %s)", t.Manager)
			}
			if t.Inactive {
				fmt.Fprintf(&b, " — NOT ACTIVATED: %s has no `%s`", rc, t.Init)
			}
			if len(t.Config) > 0 {
				fmt.Fprintf(&b, " — config: %s", strings.Join(t.Config, ", "))
			}
			if t.Docs != "" {
				fmt.Fprintf(&b, " — docs: %s", t.Docs)
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n## Not installed (install command)\n")
	switch p := inv.Platform; {
	case p.OS == "linux" && p.Brew:
		b.WriteString("Linux with Homebrew: formulae are installed with brew, the rest with the system's package manager or the tool's own installer.\n")
	case p.OS == "linux" && p.Pkg != "":
		fmt.Fprintf(&b, "Linux without Homebrew: these are installed with %s (needs sudo) or the tool's own installer. Distribution packages can be "+
			"older than the current release; say so when the version matters. Tools with no command here are left out.\n", p.Pkg)
	case p.OS == "linux":
		b.WriteString("Linux with no package manager omega knows (apt, dnf, pacman, zypper) and no Homebrew: only tools with their own installer are listed.\n")
	case !inv.Brew:
		b.WriteString("Homebrew is not installed; most of these need it (https://brew.sh).\n")
	}
	for _, t := range inv.Missing {
		fmt.Fprintf(&b, "- %s [%s]: %s — `%s`", t.Name, t.Category, t.About, t.Install)
		if t.Init != "" {
			fmt.Fprintf(&b, ", then add `%s` to %s", t.Init, rc)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n## Findings\n")
	if inv.Findings() == 0 {
		b.WriteString("Nothing stands out.\n")
	}
	for _, c := range inv.Conflicts {
		fmt.Fprintf(&b, "- %s versions are installed by several managers: %s. One is enough; the others slow the shell and make it unclear which version runs.\n",
			c.Lang, strings.Join(c.Managers, ", "))
	}
	for _, s := range inv.Shadows {
		fmt.Fprintf(&b, "- `%s` exists several times on PATH; the first one runs: %s\n", s.Bin, strings.Join(mapTilde(s.Paths), " > "))
	}
	if len(inv.PathDup) > 0 {
		fmt.Fprintf(&b, "- PATH lists these more than once: %s\n", strings.Join(mapTilde(inv.PathDup), ", "))
	}
	if len(inv.PathGone) > 0 {
		fmt.Fprintf(&b, "- PATH has folders that do not exist: %s\n", strings.Join(mapTilde(inv.PathGone), ", "))
	}
	for _, t := range inv.Inactive() {
		fmt.Fprintf(&b, "- %s is installed but not activated in the shell: add `%s` to %s\n", t.Name, t.Init, rc)
	}
	if inv.Startup > 0 {
		note := ""
		if inv.Startup > SlowStartup {
			note = " — slow: look for nvm, conda init, compinit without a cache or many plugins (zsh -i -x -c exit, or zprof)"
			if inv.Platform.Shell == "bash" {
				note = " — slow: look for nvm, conda init, bash-completion and what ~/.bashrc sources (bash -i -x -c exit)"
			}
		}
		fmt.Fprintf(&b, "- an interactive %s starts in %d ms%s\n", orShell(inv.Platform.Shell), inv.Startup.Milliseconds(), note)
	}
	if len(inv.Stores) > 0 {
		b.WriteString("\n## Installed versions per manager\n")
		for _, s := range inv.Stores {
			fmt.Fprintf(&b, "- %s: %d versions, %s in %s: %s\n", s.Name, len(s.Versions), HumanSize(s.Size), tilde(s.Dir), strings.Join(s.Versions, ", "))
		}
		b.WriteString("Offer to remove versions no project needs, with each manager's own command (nvm uninstall, fnm uninstall, pyenv uninstall, " +
			"uv python uninstall, rbenv uninstall, rustup toolchain uninstall, mise uninstall / mise prune), never with rm.\n")
	}
	return b.String()
}

func mapTilde(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = tilde(s)
	}
	return out
}

func orShell(s string) string {
	if s == "" {
		return "shell"
	}
	return s
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// InitBlock is what AddInit appends for the given tools.
func InitBlock(tools []Tool, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n# Added by omega dev (%s)\n", now.Format("2006-01-02"))
	for _, t := range tools {
		fmt.Fprintf(&b, "%s\n", t.Init)
	}
	return b.String()
}

// AddInit appends the activation lines of the tools to the shell's startup
// file (~/.zshrc, or ~/.bashrc for bash), keeping a copy of the file first. Tools that are already activated are skipped.
func AddInit(tools []Tool, now time.Time) (added []Tool, backup string, err error) {
	rc := rcText()
	for _, t := range tools {
		if t.Init != "" && !activated(t, rc) {
			added = append(added, t)
		}
	}
	if len(added) == 0 {
		return nil, "", nil
	}
	path := rcFiles()[0]
	old, readErr := os.ReadFile(path)
	if readErr == nil {
		backup = fmt.Sprintf("%s.omega-bak-%s", path, now.Format("20060102-150405"))
		if err := os.WriteFile(backup, old, 0o600); err != nil {
			return nil, "", err
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, backup, err
	}
	defer f.Close()
	if len(old) > 0 && old[len(old)-1] != '\n' {
		f.WriteString("\n")
	}
	_, err = f.WriteString(InitBlock(added, now))
	return added, backup, err
}
