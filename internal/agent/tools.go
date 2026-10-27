package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/safety"
	"github.com/lcajigasm/omega/internal/steps"
)

type handler func(ctx context.Context, a *Agent, in map[string]any) (string, error)

var handlers = map[string]handler{
	"system_info": toolSystemInfo,
	"list_steps":  toolListSteps,
	"run_step":    toolRunStep,
	"run_command": toolRunCommand,
	"disk_usage":  toolDiskUsage,
	"read_file":   toolReadFile,
	"write_file":  toolWriteFile,
	"remember":    toolRemember,
}

func str(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func toolSpecs() []llm.ToolSpec {
	return []llm.ToolSpec{
		{Name: "system_info", Description: "System summary: macOS version, architecture, disk, memory and detected package managers.",
			Properties: map[string]any{}},
		{Name: "list_steps", Description: "List the catalogue of maintenance steps (update, clean, check, backend) and whether each is available on this machine.",
			Properties: map[string]any{"category": map[string]any{"type": "string", "enum": []string{"update", "clean", "check", "backend", "all"}}}},
		{Name: "run_step", Description: "Run a catalogue step by id. Prefer this over run_command for routine actions.",
			Properties: map[string]any{"id": str("step id, e.g. brew, npm, brew-cleanup")}, Required: []string{"id"}},
		{Name: "run_command", Description: "Run a non-interactive shell command (zsh -lc). Read-only commands run directly; " +
			"commands that change the system may need the user's approval. Returns the exit code and (trimmed) output.",
			Properties: map[string]any{
				"command": str("command to run"),
				"reason":  str("why you are running it, in one sentence; shown to the user"),
			}, Required: []string{"command", "reason"}},
		{Name: "disk_usage", Description: "Show the largest subdirectories of a path.",
			Properties: map[string]any{
				"path": str("path to analyse, e.g. ~/Library/Caches"),
				"top":  map[string]any{"type": "integer", "description": "number of entries (default 15)"},
			}, Required: []string{"path"}},
		{Name: "read_file", Description: "Read a text file (configs, logs). 64 KB maximum.",
			Properties: map[string]any{"path": str("file path; ~ is supported")}, Required: []string{"path"}},
		{Name: "write_file", Description: "Write a whole configuration file. Always needs approval and keeps an .omega-bak copy of the original.",
			Properties: map[string]any{
				"path":    str("file path; ~ is supported"),
				"content": str("complete new content"),
				"reason":  str("what changes and why"),
			}, Required: []string{"path", "content", "reason"}},
		{Name: "remember", Description: "Save a lasting note about this machine for future runs (e.g. the cause of a recurring failure and its fix).",
			Properties: map[string]any{"note": str("short note, one line")}, Required: []string{"note"}},
	}
}

func getString(in map[string]any, key string) string {
	v, _ := in[key].(string)
	return strings.TrimSpace(v)
}

func expand(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	return path
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func (a *Agent) runShell(ctx context.Context, tool, command, reason string, risk safety.Risk) (string, error) {
	if ok, why := a.authorize(ctx, tool, command, reason, risk); !ok {
		return "", fmt.Errorf("%s", why)
	}
	a.emit(Event{Kind: EvToolStart, Tool: tool, Command: command, Reason: reason, Risk: risk})
	res := runner.Run(ctx, command, a.opts.Timeout, func(line string) {
		a.emit(Event{Kind: EvOutput, Tool: tool, Text: line})
	})
	a.emit(Event{Kind: EvToolEnd, Tool: tool, Command: command, ExitCode: res.ExitCode, Failed: res.ExitCode != 0, Duration: res.Duration})
	out := strings.TrimSpace(res.Output)
	if out == "" {
		out = "(no output)"
	}
	return fmt.Sprintf("exit_code: %d\nduration: %s\n---\n%s", res.ExitCode, res.Duration.Round(time.Second), out), nil
}

func toolSystemInfo(ctx context.Context, a *Agent, _ map[string]any) (string, error) {
	cmd := `sw_vers; echo "arch: $(uname -m)"; echo; df -h / | tail -1; echo; ` +
		`sysctl -n hw.memsize | awk '{printf "RAM: %.0f GB\n", $1/1073741824}'; uptime`
	a.emit(Event{Kind: EvToolStart, Tool: "system_info", Command: "system_info"})
	res := runner.Run(ctx, cmd, 30*time.Second, nil)
	var managers []string
	for _, bin := range []string{"brew", "mas", "npm", "pnpm", "bun", "pipx", "uv", "pip3", "cargo", "rustup", "gem", "go", "gh", "docker", "xcrun", "topgrade", "mo"} {
		if runner.Has(bin) {
			managers = append(managers, bin)
		}
	}
	a.emit(Event{Kind: EvToolEnd, Tool: "system_info", Command: "system_info", ExitCode: res.ExitCode})
	var b strings.Builder
	b.WriteString(strings.TrimSpace(res.Output))
	fmt.Fprintf(&b, "\n\ndetected tools: %s\n", strings.Join(managers, ", "))
	fmt.Fprintf(&b, "shell: %s, omega on %s/%s\n", runner.Shell(), runtime.GOOS, runtime.GOARCH)
	for _, f := range []string{"~/.config/topgrade.toml", "~/.config/mole"} {
		if _, err := os.Stat(expand(f)); err == nil {
			fmt.Fprintf(&b, "exists: %s\n", f)
		}
	}
	return b.String(), nil
}

func toolListSteps(_ context.Context, a *Agent, in map[string]any) (string, error) {
	cat := getString(in, "category")
	var b strings.Builder
	for _, s := range steps.All {
		if cat != "" && cat != "all" && s.Category != cat {
			continue
		}
		status := "available"
		switch {
		case a.opts.Config.Skipped(s.ID):
			status = "disabled by the user"
		case !s.Available():
			status = "not installed"
		}
		fmt.Fprintf(&b, "- %s [%s, risk %s, %s]: %s — `%s`", s.ID, s.Category, s.Risk, status, s.Name, s.Command)
		if s.Description != "" {
			b.WriteString(" — " + s.Description)
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

func toolRunStep(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	id := getString(in, "id")
	s, ok := steps.Find(id)
	if !ok {
		return "", fmt.Errorf("no step %q; use list_steps", id)
	}
	if a.opts.Config.Skipped(id) {
		return "", fmt.Errorf("the user disabled step %q in the configuration", id)
	}
	if !s.Available() {
		return "", fmt.Errorf("step %q is not available on this machine", id)
	}
	risk := s.Risk
	if safety.Classify(s.Command) == safety.Blocked {
		risk = safety.Blocked
	}
	return a.runShell(ctx, "run_step:"+id, s.Command, s.Name, risk)
}

func toolRunCommand(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	cmd := getString(in, "command")
	if cmd == "" {
		return "", fmt.Errorf("empty command")
	}
	return a.runShell(ctx, "run_command", cmd, getString(in, "reason"), safety.Classify(cmd))
}

func toolDiskUsage(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	path := expand(getString(in, "path"))
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	top := 15
	if n, ok := in["top"].(float64); ok && n > 0 && n <= 100 {
		top = int(n)
	}
	cmd := fmt.Sprintf("du -xhd 1 %s 2>/dev/null | sort -hr | head -n %d", shellQuote(path), top+1)
	return a.runShell(ctx, "disk_usage", cmd, i18n.T("agent.disk", path), safety.Safe)
}

func toolReadFile(_ context.Context, a *Agent, in map[string]any) (string, error) {
	path := expand(getString(in, "path"))
	a.emit(Event{Kind: EvToolStart, Tool: "read_file", Command: i18n.T("agent.read", path), Risk: safety.Safe})
	data, err := os.ReadFile(path)
	a.emit(Event{Kind: EvToolEnd, Tool: "read_file", Command: i18n.T("agent.read", path), Failed: err != nil})
	if err != nil {
		return "", err
	}
	const max = 64 * 1024
	if len(data) > max {
		return string(data[:max]) + "\n[… file truncated …]", nil
	}
	return string(data), nil
}

func toolWriteFile(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	path := expand(getString(in, "path"))
	content, _ := in["content"].(string)
	reason := getString(in, "reason")
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	old, readErr := os.ReadFile(path)
	summary := i18n.T("agent.write", path, len(old), len(content))
	if ok, why := a.authorize(ctx, "write_file", summary, reason+"\n\n"+diffPreview(string(old), content), safety.Dangerous); !ok {
		return "", fmt.Errorf("%s", why)
	}
	a.emit(Event{Kind: EvToolStart, Tool: "write_file", Command: summary, Risk: safety.Dangerous})
	backup := ""
	if readErr == nil {
		backup = fmt.Sprintf("%s.omega-bak-%s", path, time.Now().Format("20060102-150405"))
		if err := os.WriteFile(backup, old, 0o644); err != nil {
			return "", fmt.Errorf("could not create the backup: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	err := os.WriteFile(path, []byte(content), 0o644)
	a.emit(Event{Kind: EvToolEnd, Tool: "write_file", Command: summary, Failed: err != nil})
	if err != nil {
		return "", err
	}
	if backup != "" {
		return "written. backup at " + backup, nil
	}
	return "written (new file)", nil
}

func toolRemember(_ context.Context, a *Agent, in map[string]any) (string, error) {
	note := strings.ReplaceAll(getString(in, "note"), "\n", " ")
	if note == "" {
		return "", fmt.Errorf("empty note")
	}
	if err := os.MkdirAll(config.Dir(), 0o755); err != nil {
		return "", err
	}
	f, err := os.OpenFile(config.NotesPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	fmt.Fprintf(f, "- %s: %s\n", time.Now().Format("2006-01-02"), note)
	a.emit(Event{Kind: EvToolEnd, Tool: "remember", Command: i18n.T("agent.note_saved", note)})
	return "saved", nil
}

// diffPreview shows changed lines between two versions, capped for display.
func diffPreview(old, new string) string {
	ol := strings.Split(old, "\n")
	nl := strings.Split(new, "\n")
	inOld := map[string]int{}
	for _, l := range ol {
		inOld[l]++
	}
	inNew := map[string]int{}
	for _, l := range nl {
		inNew[l]++
	}
	var b strings.Builder
	n := 0
	for _, l := range ol {
		if inNew[l] == 0 && n < 40 {
			b.WriteString("- " + l + "\n")
			n++
		}
	}
	for _, l := range nl {
		if inOld[l] == 0 && n < 40 {
			b.WriteString("+ " + l + "\n")
			n++
		}
	}
	if n == 0 {
		return "(no line changes)"
	}
	return strings.TrimRight(b.String(), "\n")
}
