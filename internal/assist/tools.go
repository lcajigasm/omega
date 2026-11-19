package assist

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The tools, in the order they are shown. Every setting below is one the
// tool documents; where it has none, the check is a budget or a cleanup.

func init() {
	register("claude-code", claudeCode)
	register("claude-desktop", claudeDesktop)
	register("codex", codex)
	register("grok", grok)
	register("docker", docker)
	register("cursor", cursor)
	register("vscode", vscode)
	register("gemini-cli", geminiCLI)
	register("homebrew", homebrew)
	register("xcode", xcode)
	register("ollama", ollama)
}

// ---------- Claude Code ----------

// secretRules keep Claude Code from reading the files where keys live.
var secretRules = []string{"Read(./.env)", "Read(./.env.*)", "Read(./secrets/**)", "Read(~/.ssh/**)", "Read(~/.aws/**)", "Read(~/.config/omega/**)"}

func claudeCode(e *env) Tool {
	t := Tool{Name: "Claude Code", Kind: "cli", About: "Anthropic's coding agent in the terminal (claude)."}
	dir := e.path(".claude")
	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" && e.home == homeDir() {
		dir = v
	}
	bin, hasBin := e.has("claude")
	t.Installed = hasBin || isDir(dir)
	if !t.Installed {
		return t
	}
	if hasBin {
		t.Version = cliVersion(bin)
	}
	t.Where = dir
	t.DataSize = size(dir)
	settings := filepath.Join(dir, "settings.json")

	// Transcripts: cleanupPeriodDays, 30 by default.
	cur := 30
	if v := jsonGet(settings, "cleanupPeriodDays"); v.Exists() {
		cur = int(v.Int())
	}
	const rec = 14
	transcripts := Check{ID: "claude-code.transcripts", Title: "Days Claude Code keeps conversations", Kind: KindSetting, Disk: true,
		Why:     "Every session is saved in ~/.claude/projects and kept for cleanupPeriodDays (30 by default); `claude --resume` only needs the recent ones.",
		Size:    size(filepath.Join(dir, "projects"), filepath.Join(dir, "todos"), filepath.Join(dir, "shell-snapshots"), filepath.Join(dir, "file-history")),
		Current: fmt.Sprintf("%d days", cur), Recommended: strconv.Itoa(rec), Unit: "days", OK: cur <= rec, File: settings,
		Note: "Claude Code removes older conversations the next time it starts."}
	transcripts.set = func(v string) (string, error) {
		n, err := days(v, 1, 3650)
		if err != nil {
			return "", err
		}
		return jsonSet("claude-code", settings, fmt.Sprintf("Claude Code keeps conversations %d days", n), map[string]any{"cleanupPeriodDays": n})
	}
	t.Checks = append(t.Checks, transcripts)

	// Keys out of reach.
	have := map[string]bool{}
	for _, r := range jsonGet(settings, "permissions/deny").Array() {
		have[r.String()] = true
	}
	var missing []string
	for _, r := range secretRules {
		if !have[r] {
			missing = append(missing, r)
		}
	}
	deny := Check{ID: "claude-code.secrets", Title: "Keep Claude Code out of .env files and keys", Kind: KindAction, File: settings, OK: len(missing) == 0,
		Why:         "Deny rules stop the agent from reading .env files, SSH and AWS keys, and omega's own configuration, whatever a prompt or a repository asks.",
		Recommended: strings.Join(secretRules, " ")}
	if deny.OK {
		deny.Current = "all denied"
	} else {
		deny.Current = "missing: " + strings.Join(missing, " ")
	}
	deny.set = func(string) (string, error) {
		return jsonAppend("claude-code", settings, "permissions/deny", "deny rules for secrets", secretRules)
	}
	t.Checks = append(t.Checks, deny)

	// omega's MCP server, user scope, through the claude command so its own
	// file stays its business.
	userFile := e.path(".claude.json")
	present := jsonGet(userFile, "mcpServers/omega").Exists()
	mcp := Check{ID: "claude-code.mcp", Title: "Give Claude Code omega's tools (MCP)", Kind: KindAction, OK: present, File: userFile, Recommended: "register",
		Why:     "Lets the agent check the disk, the limits and these assistants through omega, read its manual and change settings with your approval, instead of improvising commands.",
		Current: map[bool]string{true: "registered", false: "not registered"}[present]}
	if !hasBin {
		mcp.Blocked = "the claude command is not installed"
	}
	mcp.set = func(string) (string, error) {
		out, err := run(bin, "mcp", "add", "--scope", "user", "omega", "--", e.omega, "mcp")
		if err != nil {
			return "", fmt.Errorf("claude mcp add: %v: %s", err, out)
		}
		return "omega's MCP server added to Claude Code (user scope): " + out, nil
	}
	t.Checks = append(t.Checks, mcp)

	// The native installer keeps every version it downloaded.
	versions := e.path(".local", "share", "claude", "versions")
	if isDir(versions) {
		old := oldVersions(versions, e.path(".local", "bin", "claude"))
		c := cleanup("claude-code.versions", "Old Claude Code versions", "The native installer keeps each version it updated from; only the one ~/.local/bin/claude points to runs.", versions, old)
		t.Checks = append(t.Checks, c)
	}
	return t
}

// ---------- Claude Desktop ----------

func claudeDesktop(e *env) Tool {
	t := Tool{Name: "Claude Desktop", Kind: "app", About: "The Claude app: chat, Cowork and Claude Code in a window."}
	bundle, hasApp := e.app("Claude")
	data := e.appSupport("Claude")
	t.Installed = hasApp || isDir(data)
	if !t.Installed {
		return t
	}
	t.Version = appVersion(bundle)
	t.Running = e.appRunning(bundle)
	t.Where = data
	t.DataSize = size(data)

	vm := filepath.Join(data, "vm_bundles")
	if isDir(vm) {
		t.Checks = append(t.Checks, budget(e, "claude-desktop.vm", "Cowork virtual machine image",
			"Claude Desktop downloads a Linux VM image to run Cowork and code tasks in a sandbox and never trims it; when it is missing it downloads it again on the next task.",
			data, false, vm))
	}
	t.Checks = append(t.Checks, caches("claude-desktop.cache", "Claude Desktop", data))
	if cc := filepath.Join(data, "claude-code"); isDir(cc) {
		t.Checks = append(t.Checks, cleanup("claude-desktop.versions", "Old Claude Code versions inside Claude Desktop",
			"The app downloads its own Claude Code for each version and keeps the earlier ones.", data, oldVersions(cc)))
	}
	cfg := filepath.Join(data, "claude_desktop_config.json")
	t.Checks = append(t.Checks, mcpJSON(e, "claude-desktop.mcp", "Claude Desktop", cfg, "mcpServers/omega", mcpEntry(e), "Quit and open Claude Desktop again to load it."))
	return t
}

// ---------- ChatGPT and Codex ----------

func codex(e *env) Tool {
	t := Tool{Name: "ChatGPT / Codex", Kind: "both", About: "OpenAI's ChatGPT app and the Codex CLI. They share ~/.codex."}
	dir := e.path(".codex")
	if v := os.Getenv("CODEX_HOME"); v != "" && e.home == homeDir() {
		dir = v
	}
	bin, hasBin := e.has("codex")
	bundle, hasApp := e.app("ChatGPT")
	t.Installed = hasBin || hasApp || isDir(dir)
	if !t.Installed {
		return t
	}
	switch {
	case hasBin:
		t.Version = cliVersion(bin)
	case hasApp:
		t.Version = "app " + appVersion(bundle)
	}
	t.Running = e.appRunning(bundle)
	t.Where = dir
	t.DataSize = size(dir)
	cfg := filepath.Join(dir, "config.toml")

	// history.jsonl: [history] max_bytes, unbounded by default.
	cur := "no limit"
	ok := false
	if v, isInt := tomlGet(cfg, "history", "max_bytes").(int64); isInt && v > 0 {
		cur = Human(v)
		ok = v <= 50<<20
	}
	if p, isStr := tomlGet(cfg, "history", "persistence").(string); isStr && p == "none" {
		cur, ok = "not saved", true
	}
	hist := Check{ID: "codex.history", Title: "Size of Codex's prompt history", Kind: KindSetting, Disk: true, File: cfg, Unit: "MB",
		Why:  "Codex appends every prompt to history.jsonl and never trims it unless [history] max_bytes is set.",
		Size: size(filepath.Join(dir, "history.jsonl")), Current: cur, Recommended: "20MB", OK: ok}
	hist.set = func(v string) (string, error) {
		n, err := ParseSize(v)
		if err != nil {
			return "", err
		}
		if n < 1<<20 {
			return "", fmt.Errorf("%s is too small for a history", Human(n))
		}
		return tomlSet("codex", cfg, "history", [][2]string{{"max_bytes", strconv.FormatInt(n, 10)}}, "Codex history limited to "+Human(n))
	}
	t.Checks = append(t.Checks, hist)

	t.Checks = append(t.Checks, budget(e, "codex.sessions", "Codex session recordings",
		"Every Codex session is recorded in ~/.codex/sessions, and Codex never removes them; `codex resume` only needs the recent ones.",
		dir, true, filepath.Join(dir, "sessions"), filepath.Join(dir, "archived_sessions")))

	present := tomlGet(cfg, "mcp_servers", "omega") != nil
	mcp := Check{ID: "codex.mcp", Title: "Give ChatGPT and Codex omega's tools (MCP)", Kind: KindAction, File: cfg, OK: present, Recommended: "register",
		Why:     "Lets the agent check the disk, the limits and these assistants through omega, read its manual and change settings with your approval, instead of improvising commands.",
		Current: map[bool]string{true: "registered", false: "not registered"}[present]}
	mcp.set = func(string) (string, error) {
		return tomlSet("codex", cfg, "mcp_servers.omega", [][2]string{{"command", strconv.Quote(e.omega)}, {"args", `["mcp"]`}}, "omega's MCP server for Codex")
	}
	t.Checks = append(t.Checks, mcp)

	// The ChatGPT app keeps a browser profile for its agent.
	for _, name := range []string{"Codex", "com.openai.chat"} {
		if d := e.appSupport(name); isDir(d) && len(cacheDirs(d)) > 0 {
			t.Checks = append(t.Checks, caches("codex.cache", "ChatGPT app", d))
			break
		}
	}
	return t
}

// ---------- Grok ----------

func grok(e *env) Tool {
	t := Tool{Name: "Grok", Kind: "both", About: "xAI's Grok desktop app (Grok Bot) and the grok command."}
	var bundle, data string
	for _, name := range []string{"Grok Bot", "Grok"} {
		if b, ok := e.app(name); ok {
			bundle = b
			data = e.appSupport(name)
			break
		}
	}
	bin, hasBin := e.has("grok")
	cliDir := e.path(".grok")
	t.Installed = bundle != "" || hasBin || isDir(cliDir)
	if !t.Installed {
		return t
	}
	if bundle != "" {
		t.Version = "app " + appVersion(bundle)
		t.Running = e.appRunning(bundle)
		t.Where = data
		t.DataSize = size(data)
		c := caches("grok.cache", "Grok", data)
		// Its own caches of previews and images.
		for _, extra := range []string{"link-preview-cache", "attachment-image-cache", "link-favicon-cache", "plugin-logo-cache"} {
			if p := filepath.Join(data, extra); isDir(p) {
				c.Size += size(p)
			}
		}
		c.Free = c.Size
		c.OK = c.Size < 100<<20
		c.Current = Human(c.Size)
		dirs := append(cacheDirs(data), existing(data, "link-preview-cache", "attachment-image-cache", "link-favicon-cache", "plugin-logo-cache")...)
		c.clean = func() (string, error) {
			freed, err := emptyDirs(data, dirs...)
			if err != nil {
				return "", err
			}
			return "Grok caches: freed " + Human(freed), nil
		}
		t.Checks = append(t.Checks, c)
	}
	if hasBin || isDir(cliDir) {
		if hasBin && t.Version == "" {
			t.Version = cliVersion(bin)
		}
		if t.Where == "" {
			t.Where = cliDir
		}
		t.DataSize += size(cliDir)
		info := Check{ID: "grok.cli", Title: "grok command", Kind: KindInfo, OK: true, Current: map[bool]string{true: "installed: " + bin, false: "settings only in ~/.grok"}[hasBin],
			Why: "Its settings live in ~/.grok/user-settings.json; omega reads nothing from them."}
		t.Checks = append(t.Checks, info)
	}
	return t
}

func existing(base string, names ...string) []string {
	var out []string
	for _, n := range names {
		if p := filepath.Join(base, n); isDir(p) {
			out = append(out, p)
		}
	}
	return out
}

// ---------- Docker ----------

func docker(e *env) Tool {
	t := Tool{Name: "Docker", Kind: "both", About: "Docker Desktop and the docker command."}
	bundle, hasApp := e.app("Docker")
	bin, hasBin := e.has("docker")
	t.Installed = hasApp || hasBin
	if !t.Installed {
		return t
	}
	if hasApp {
		t.Version = "Desktop " + appVersion(bundle)
		t.Running = e.appRunning(bundle)
	} else {
		t.Version = cliVersion(bin)
	}
	daemon := e.path(".docker", "daemon.json")
	t.Where = daemon

	// Build cache: builder.gc keeps it under defaultKeepStorage.
	keep := jsonGet(daemon, "builder/gc/defaultKeepStorage").String()
	enabled := jsonGet(daemon, "builder/gc/enabled")
	ok := keep != "" && (!enabled.Exists() || enabled.Bool())
	cur := "no limit"
	if keep != "" {
		cur = "keep " + keep
		if enabled.Exists() && !enabled.Bool() {
			cur += " (garbage collection off)"
		}
	}
	bc := Check{ID: "docker.build-cache", Title: "Docker build cache limit", Kind: KindSetting, Disk: true, File: daemon, Unit: "GB",
		Why:     "BuildKit keeps every layer it built. With builder.gc on, Docker trims the cache down to defaultKeepStorage by itself.",
		Current: cur, Recommended: "10GB", OK: ok, Note: "Docker applies it when the engine restarts (Docker Desktop: Restart)."}
	bc.set = func(v string) (string, error) {
		n, err := ParseSize(v)
		if err != nil {
			return "", err
		}
		return jsonSet("docker", daemon, "Docker build cache kept under "+dockerSize(n), map[string]any{"builder/gc/enabled": true, "builder/gc/defaultKeepStorage": dockerSize(n)})
	}
	t.Checks = append(t.Checks, bc)

	// Container logs: json-file without max-size grows for ever.
	driver := jsonGet(daemon, "log-driver").String()
	maxSize := jsonGet(daemon, "log-opts/max-size").String()
	logs := Check{ID: "docker.logs", Title: "Size of container logs", Kind: KindSetting, Disk: true, File: daemon, Unit: "MB per container",
		Why:         "With the default json-file driver a container's log grows without end. max-size and max-file rotate it.",
		Recommended: "10m", Note: "It applies to containers created after Docker restarts."}
	switch {
	case driver != "" && driver != "json-file":
		logs.Current, logs.OK = "log driver "+driver, true
	case maxSize != "":
		logs.Current, logs.OK = fmt.Sprintf("max %s × %s files", maxSize, orDefault(jsonGet(daemon, "log-opts/max-file").String(), "1")), true
	default:
		logs.Current = "no limit"
	}
	logs.set = func(v string) (string, error) {
		v = strings.ToLower(strings.TrimSpace(v))
		v = strings.TrimSuffix(strings.TrimSuffix(v, "b"), "i")
		if _, err := strconv.Atoi(strings.TrimRight(v, "kmg")); err != nil || !strings.ContainsAny(v[len(v)-1:], "kmg") {
			return "", fmt.Errorf("%q: give a size such as 10m or 50m", v)
		}
		return jsonSet("docker", daemon, "Docker logs rotated at "+v, map[string]any{"log-driver": "json-file", "log-opts/max-size": v, "log-opts/max-file": "3"})
	}
	t.Checks = append(t.Checks, logs)

	// Docker Desktop's disk image on macOS.
	if hasApp {
		store := e.path("Library", "Group Containers", "group.com.docker", "settings-store.json")
		vmData := e.path("Library", "Containers", "com.docker.docker", "Data", "vms", "0", "data")
		used := size(vmData)
		t.DataSize = used
		disk := Check{ID: "docker.disk", Title: "Docker Desktop disk image size", Kind: KindSetting, Disk: true, File: store, Unit: "GB", Size: used,
			Why:  "Images, containers, volumes and the build cache all live in one disk image. Its size is the most Docker can ever take.",
			Note: "Docker Desktop reads it when it starts. Going below what it holds now makes it ask to recreate the image, which deletes images, containers and volumes: omega refuses that."}
		cur := jsonGet(store, "DiskSizeMiB")
		rec := max(int64(32)<<30, roundUp(used*2, 8<<30))
		disk.Recommended = Human(rec)
		if cur.Exists() && cur.Int() > 0 {
			limit := cur.Int() << 20
			disk.Current = Human(limit)
			disk.OK = limit <= 2*rec
		} else {
			disk.Current = "Docker's default"
		}
		if t.Running {
			disk.Blocked = "Docker Desktop is open: quit it first, it writes this file"
		}
		disk.set = func(v string) (string, error) {
			n, err := ParseSize(v)
			if err != nil {
				return "", err
			}
			if n < used+2<<30 {
				return "", fmt.Errorf("Docker's disk holds %s: a limit below %s would make Docker Desktop delete it", Human(used), Human(used+2<<30))
			}
			return jsonSet("docker", store, "Docker Desktop disk limited to "+Human(n), map[string]any{"DiskSizeMiB": n >> 20})
		}
		t.Checks = append(t.Checks, disk)
	}
	return t
}

func dockerSize(n int64) string {
	if n%(1<<30) == 0 {
		return fmt.Sprintf("%dGB", n>>30)
	}
	return fmt.Sprintf("%dMB", n>>20)
}

func roundUp(n, step int64) int64 { return (n + step - 1) / step * step }

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// ---------- Cursor ----------

func cursor(e *env) Tool {
	t := Tool{Name: "Cursor", Kind: "both", About: "The Cursor editor and its agent CLI (cursor-agent)."}
	bundle, hasApp := e.app("Cursor")
	data := e.appSupport("Cursor")
	cliVersions := e.path(".local", "share", "cursor-agent", "versions")
	_, hasCLI := e.has("cursor-agent")
	t.Installed = hasApp || isDir(data) || hasCLI || isDir(cliVersions)
	if !t.Installed {
		return t
	}
	t.Version = appVersion(bundle)
	t.Running = e.appRunning(bundle)
	t.Where = data
	t.DataSize = size(data)

	worker := filepath.Join(data, "User", "globalStorage", "anysphere.cursor-agent-worker", "agent-cli")
	if isDir(worker) {
		old := oldVersions(filepath.Join(worker, ".local", "share", "cursor-agent", "versions"), filepath.Join(worker, ".local", "bin", "cursor-agent"))
		tars, _ := filepath.Glob(filepath.Join(worker, "*.tar.gz"))
		t.Checks = append(t.Checks, cleanup("cursor.agent-versions", "Old versions of Cursor's background agent",
			"Cursor downloads its agent for every update and keeps every earlier version and the downloaded archives; only the one its link points to runs.",
			data, append(old, tars...)))
	}
	if isDir(cliVersions) {
		old := oldVersions(cliVersions, e.path(".local", "bin", "cursor-agent"), e.path(".local", "bin", "agent"))
		t.Checks = append(t.Checks, cleanup("cursor.cli-versions", "Old versions of the cursor-agent command",
			"The installer keeps each version it updated from; only the one ~/.local/bin/cursor-agent points to runs.",
			filepath.Dir(cliVersions), old))
	}
	if isDir(data) {
		t.Checks = append(t.Checks, caches("cursor.cache", "Cursor", data))
	}
	t.Checks = append(t.Checks, mcpJSON(e, "cursor.mcp", "Cursor", e.path(".cursor", "mcp.json"), "mcpServers/omega", mcpEntry(e), "Cursor picks it up in Settings → MCP."))
	return t
}

// ---------- VS Code ----------

func vscode(e *env) Tool {
	t := Tool{Name: "VS Code", Kind: "app", About: "Visual Studio Code, with Copilot's agent mode."}
	bundle, hasApp := e.app("Visual Studio Code")
	_, hasBin := e.has("code")
	data := e.appSupport("Code")
	t.Installed = hasApp || hasBin && isDir(data)
	if !t.Installed {
		return t
	}
	t.Version = appVersion(bundle)
	t.Running = e.appRunning(bundle)
	t.Where = data
	t.DataSize = size(data)
	if isDir(data) {
		t.Checks = append(t.Checks, caches("vscode.cache", "VS Code", data))
	}
	entry := map[string]any{"type": "stdio", "command": e.omega, "args": []string{"mcp"}}
	t.Checks = append(t.Checks, mcpJSON(e, "vscode.mcp", "VS Code", filepath.Join(data, "User", "mcp.json"), "servers/omega", entry, "VS Code asks you to trust the server the first time it starts it."))
	return t
}

// ---------- Gemini CLI ----------

func geminiCLI(e *env) Tool {
	t := Tool{Name: "Gemini CLI", Kind: "cli", About: "Google's coding agent in the terminal (gemini)."}
	dir := e.path(".gemini")
	bin, hasBin := e.has("gemini")
	t.Installed = hasBin || isDir(dir)
	if !t.Installed {
		return t
	}
	if hasBin {
		t.Version = cliVersion(bin)
	}
	t.Where = dir
	t.DataSize = size(dir)
	if tmp := filepath.Join(dir, "tmp"); isDir(tmp) {
		t.Checks = append(t.Checks, budget(e, "gemini-cli.tmp", "Gemini CLI chats and checkpoints",
			"Saved chats, checkpoints and shell history per project in ~/.gemini/tmp.", dir, true, tmp))
	}
	t.Checks = append(t.Checks, mcpJSON(e, "gemini-cli.mcp", "Gemini CLI", filepath.Join(dir, "settings.json"), "mcpServers/omega", mcpEntry(e), "Gemini CLI loads it the next time it starts."))
	return t
}

// ---------- Homebrew ----------

func homebrew(e *env) Tool {
	t := Tool{Name: "Homebrew", Kind: "cli", About: "The package manager for macOS and Linux (brew)."}
	bin, ok := e.has("brew")
	t.Installed = ok
	if !ok {
		return t
	}
	env := e.path(".homebrew", "brew.env")
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" && e.home == homeDir() {
		env = filepath.Join(x, "homebrew", "brew.env")
	}
	cache := e.path(".cache", "Homebrew")
	if e.goos == "darwin" {
		cache = e.path("Library", "Caches", "Homebrew")
	}
	t.Where = env
	t.DataSize = size(cache)
	cur := envGet(env, "HOMEBREW_CLEANUP_MAX_AGE_DAYS")
	if cur == "" {
		cur = os.Getenv("HOMEBREW_CLEANUP_MAX_AGE_DAYS")
	}
	n, _ := strconv.Atoi(cur)
	if n == 0 {
		n = 120
	}
	age := Check{ID: "homebrew.cleanup-days", Title: "Days Homebrew keeps downloads", Kind: KindSetting, Disk: true, File: env, Unit: "days", Size: t.DataSize,
		Why:     "brew cleanup, which runs on its own every month, removes downloads older than HOMEBREW_CLEANUP_MAX_AGE_DAYS (120 by default).",
		Current: fmt.Sprintf("%d days", n), Recommended: "30", OK: n <= 30}
	age.set = func(v string) (string, error) {
		d, err := days(v, 1, 3650)
		if err != nil {
			return "", err
		}
		return envSet("homebrew", env, "HOMEBREW_CLEANUP_MAX_AGE_DAYS", strconv.Itoa(d), fmt.Sprintf("Homebrew keeps downloads %d days", d))
	}
	t.Checks = append(t.Checks, age)

	clean := Check{ID: "homebrew.downloads", Title: "Homebrew's downloads and old versions", Kind: KindCleanup, Disk: true, Size: t.DataSize, Free: t.DataSize, OK: t.DataSize < 200<<20,
		Why: "brew cleanup --prune=all removes every cached download and the versions upgrades left installed.", Current: Human(t.DataSize)}
	clean.clean = func() (string, error) {
		out, err := run(bin, "cleanup", "--prune=all")
		if err != nil {
			return "", fmt.Errorf("brew cleanup: %v: %s", err, out)
		}
		return "brew cleanup --prune=all: freed " + Human(t.DataSize-size(cache)), nil
	}
	t.Checks = append(t.Checks, clean)
	return t
}

// ---------- Xcode ----------

func xcode(e *env) Tool {
	t := Tool{Name: "Xcode", Kind: "app", About: "Apple's IDE: build products and simulators."}
	bundle, ok := e.app("Xcode")
	t.Installed = ok
	if !ok {
		return t
	}
	t.Version = appVersion(bundle)
	t.Running = e.appRunning(bundle)
	dev := e.path("Library", "Developer")
	t.Where = dev
	derived := filepath.Join(dev, "Xcode", "DerivedData")
	t.Checks = append(t.Checks, budget(e, "xcode.derived-data", "Xcode DerivedData",
		"Build products and indexes of every project ever opened. Xcode builds them again; it never removes those of old projects.", dev, false, derived))
	if sims := filepath.Join(dev, "CoreSimulator", "Devices"); isDir(sims) {
		t.DataSize = size(derived, sims)
		t.Checks = append(t.Checks, Check{ID: "xcode.simulators", Title: "Simulator devices", Kind: KindInfo, Disk: true, OK: true, Size: size(sims),
			Current: Human(size(sims)), Why: "`xcrun simctl delete unavailable` removes the devices of runtimes no longer installed; omega's maintenance runs it."})
	}
	return t
}

// ---------- Ollama ----------

func ollama(e *env) Tool {
	t := Tool{Name: "Ollama", Kind: "both", About: "Local models with Ollama."}
	_, hasBin := e.has("ollama")
	_, hasApp := e.app("Ollama")
	models := e.path(".ollama", "models")
	if v := os.Getenv("OLLAMA_MODELS"); v != "" && e.home == homeDir() {
		models = v
	}
	t.Installed = hasBin || hasApp || isDir(models)
	if !t.Installed {
		return t
	}
	t.Where = models
	t.DataSize = size(models)
	t.Checks = append(t.Checks, Check{ID: "ollama.models", Title: "Downloaded models", Kind: KindInfo, Disk: true, OK: true, Size: t.DataSize, Current: Human(t.DataSize),
		Why: "Each model is gigabytes. `ollama list` shows them and `ollama rm <model>` removes one; omega does not choose for you."})
	return t
}
