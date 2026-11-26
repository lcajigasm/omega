// Package demo provides a scripted model and fake command runner so omega's
// interface can be shown (and screenshotted) without API calls or touching
// the machine. All data in it is made up.
package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/runner"
)

// Provider returns the scripted provider.
func Provider() llm.Provider { return &provider{} }

type provider struct{}

func (provider) Name() string                             { return "anthropic" }
func (provider) Models(context.Context) ([]string, error) { return nil, nil }
func (provider) NewChat(_ string, _ string, specs []llm.ToolSpec) llm.Chat {
	for _, s := range specs {
		if s.Name == "plan_actions" {
			// The stage is known from the task text of the first turn.
			return &chat{audit: "audit"}
		}
	}
	return &chat{}
}

type chat struct {
	turn  int
	picks []string
	n     int
	// audit is the stage of omega ultra's audit this chat plays, "" for
	// an ordinary session.
	audit string
}

func (c *chat) SetModel(string) {}

func call(c *chat, name string, in map[string]any) llm.ToolCall {
	c.n++
	b, _ := json.Marshal(in)
	return llm.ToolCall{ID: fmt.Sprintf("call_%d", c.n), Name: name, Input: b}
}

func step(c *chat, id string) llm.ToolCall { return call(c, "run_step", map[string]any{"id": id}) }

func (c *chat) Send(ctx context.Context, text string, results []llm.ToolResult) (*llm.Reply, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(1400 * time.Millisecond):
	}
	for _, r := range results {
		if strings.HasPrefix(r.Content, "The user selected:") {
			for _, line := range strings.Split(r.Content, "\n") {
				if strings.HasPrefix(line, "- ") {
					c.picks = append(c.picks, strings.SplitN(strings.TrimPrefix(line, "- "), ":", 2)[0])
				}
			}
		}
	}
	c.turn++
	u := llm.Usage{InputTokens: 3200 + int64(c.turn)*900, OutputTokens: 180 + int64(c.turn)*40}
	r := &llm.Reply{Usage: u}
	if c.audit != "" {
		if c.turn == 1 {
			c.audit = auditStage(text)
		}
		return c.auditTurn(r), nil
	}
	switch c.turn {
	case 1:
		r.Calls = []llm.ToolCall{call(c, "system_info", map[string]any{})}
	case 2:
		r.Calls = []llm.ToolCall{step(c, "disk"), step(c, "brew-outdated")}
	case 3:
		r.Text = "Disk is at **91%** and 6 Homebrew packages are outdated. Updating first."
		r.Calls = []llm.ToolCall{step(c, "brew")}
	case 4:
		r.Calls = []llm.ToolCall{step(c, "pipx"), step(c, "npm")}
	case 5:
		r.Text = "`pipx` failed because its environments still point at **Python 3.12**, which Homebrew replaced with 3.13. Reinstalling them fixes it."
		r.Calls = []llm.ToolCall{call(c, "run_command", map[string]any{
			"command": "pipx reinstall-all --python python3.13",
			"reason":  "Rebuild the pipx environments against the current Homebrew Python",
		})}
	case 6:
		r.Calls = []llm.ToolCall{call(c, "disk_usage", map[string]any{"path": "~/Library", "top": 6})}
	case 7:
		r.Text = "Everything is up to date. These are the biggest wins for disk space:"
		r.Calls = []llm.ToolCall{call(c, "propose_actions", map[string]any{
			"question": "What should I clean up?",
			"multiple": true,
			"options": []map[string]any{
				{"id": "brew-cleanup", "label": "Clear the Homebrew cache", "detail": "Old versions and downloads. Safe.", "impact": "~8.9 GB", "risk": "normal"},
				{"id": "docker-build-cache", "label": "Docker build cache", "detail": "Next builds will be slower.", "impact": "~6.2 GB", "risk": "normal"},
				{"id": "xcode-derived", "label": "Xcode DerivedData", "detail": "Rebuilt automatically by Xcode.", "impact": "~4.1 GB", "risk": "normal"},
				{"id": "npm-cache", "label": "npm cache", "detail": "Packages are downloaded again when needed.", "impact": "~1.3 GB", "risk": "normal"},
			},
		})}
	default:
		if len(c.picks) > 0 {
			id := c.picks[0]
			c.picks = c.picks[1:]
			r.Calls = []llm.ToolCall{step(c, id)}
			return r, nil
		}
		r.Text = "**All done.**\n\n- **Updated:** 6 Homebrew packages and 2 npm globals\n- **Fixed:** pipx, rebuilt on Python 3.13\n- **Freed:** 16.4 GB · disk now at 87%\n\nI saved a note about the pipx fix, so next time it is applied straight away."
	}
	return r, nil
}

var outputs = []struct {
	match string
	exit  int
	lines []string
}{
	{"sw_vers", 0, []string{"ProductName: macOS", "ProductVersion: 27.2", "arch: arm64", "/dev/disk3s1s1  460Gi  418Gi  42Gi  91%  /"}},
	// The demo is a recorded Mac session wherever it is played: off macOS
	// system_info runs another command and gets the same canned answer.
	{"/etc/os-release", 0, []string{"ProductName: macOS", "ProductVersion: 27.2", "arch: arm64", "/dev/disk3s1s1  460Gi  418Gi  42Gi  91%  /"}},
	{"df -h", 0, []string{"Filesystem      Size   Used  Avail Capacity  Mounted on", "/dev/disk3s1s1  460Gi  418Gi   42Gi    91%    /", " 14G  ~/Library/Caches", "1.1G  ~/Library/Logs"}},
	{"brew outdated", 0, []string{"node (24.9.0) < 24.11.1", "gh (2.81.0) < 2.83.0", "ffmpeg (8.0) < 8.0.1", "go (1.27.2) < 1.27.3", "uv (0.9.4) < 0.9.7", "ripgrep (15.0.0) < 15.1.0"}},
	{"brew update", 0, []string{"==> Updating Homebrew...", "Already up-to-date.", "==> Upgrading 6 outdated packages:", "node 24.9.0 -> 24.11.1", "gh 2.81.0 -> 2.83.0", "==> Pouring node--24.11.1.arm64_sequoia.bottle.tar.gz", "🍺  /opt/homebrew/Cellar/node/24.11.1: 2,145 files, 78.4MB", "==> Pouring ffmpeg--8.0.1.arm64_sequoia.bottle.tar.gz", "🍺  /opt/homebrew/Cellar/ffmpeg/8.0.1: 285 files, 54.1MB"}},
	{"pipx upgrade-all", 1, []string{"Upgrading httpie...", "Error: python@3.12 is not installed", "  The interpreter for 4 venvs no longer exists.", "Error upgrading 4 packages"}},
	{"npm update -g", 0, []string{"changed 2 packages in 3s"}},
	{"pipx reinstall-all", 0, []string{"uninstalled httpie", "installed package httpie 3.2.4, installed using Python 3.13.9", "installed package poetry 2.2.1, installed using Python 3.13.9", "installed package ruff 0.14.2, installed using Python 3.13.9", "installed package black 25.9.0, installed using Python 3.13.9"}},
	{"du -xhd", 0, []string{"118G  ~/Library", " 41G  ~/Library/Containers", " 22G  ~/Library/Developer", " 14G  ~/Library/Caches", "9.8G  ~/Library/Application Support", "3.1G  ~/Library/Mail"}},
	{"brew cleanup", 0, []string{"Removing: /opt/homebrew/Cellar/node/24.9.0... (2,141 files, 77.9MB)", "Removing: ~/Library/Caches/Homebrew/downloads/... (412 files, 6.1GB)", "==> This operation has freed approximately 8.9GB of disk space."}},
	{"docker builder prune", 0, []string{"ID              RECLAIMABLE  SIZE    LAST ACCESSED", "Total:  6.2GB"}},
	{"DerivedData", 0, nil},
	{"npm cache clean", 0, []string{"npm warn using --force Recommended protections disabled."}},
}

// Exec is a fake command runner that streams canned output.
func Exec(ctx context.Context, command string, _ time.Duration, onLine func(string)) runner.Result {
	start := time.Now()
	for _, o := range outputs {
		if !strings.Contains(command, o.match) {
			continue
		}
		var all []string
		for _, l := range o.lines {
			select {
			case <-ctx.Done():
				return runner.Result{ExitCode: -1, Duration: time.Since(start)}
			case <-time.After(280 * time.Millisecond):
			}
			all = append(all, l)
			if onLine != nil {
				onLine(l)
			}
		}
		time.Sleep(400 * time.Millisecond)
		return runner.Result{ExitCode: o.exit, Output: strings.Join(all, "\n"), Duration: time.Since(start) + 2*time.Second}
	}
	time.Sleep(500 * time.Millisecond)
	return runner.Result{Output: "ok", Duration: time.Since(start)}
}

// auditStage names the omega ultra stage a task text starts.
func auditStage(text string) string {
	switch {
	case strings.Contains(text, "Deep clean"):
		return "deep"
	case strings.Contains(text, "every review omega has"):
		return "care"
	case strings.HasPrefix(text, "Update"):
		return "update"
	case strings.HasPrefix(text, "Clean"), strings.HasPrefix(text, "Free"):
		return "clean"
	}
	return "other"
}

// demoActions are the actions each scripted audit stage plans.
var demoActions = map[string][]map[string]any{
	"update": {
		{"id": "brew-upgrade", "category": "Updates", "title": "Update 6 Homebrew packages",
			"why":    "brew outdated lists 6 packages behind: node 24.9.0 → 24.11.1 (security fixes), gh, ffmpeg, go, uv and ripgrep.",
			"what":   "Homebrew downloads and installs the new versions; open terminals keep the old ones until restarted. The previous versions stay until the next cleanup.",
			"impact": "6 packages", "risk": "normal", "recommended": true, "reversible": true,
			"evidence": "brew outdated: 6 formulae, no casks; brew doctor reports nothing blocking an upgrade",
			"steps":    []map[string]any{{"tool": "run_step", "input": map[string]any{"id": "brew"}}}},
		{"id": "pipx-python", "category": "Repairs", "title": "Rebuild 4 pipx tools on Python 3.13",
			"why":    "pipx upgrade-all fails: httpie, poetry, ruff and black still point at Python 3.12, which Homebrew replaced with 3.13.",
			"what":   "pipx reinstalls the 4 tools on the current Python; their settings are kept. Until then those commands fail to start.",
			"impact": "fixes 4 commands", "risk": "normal", "recommended": true,
			"evidence": "pipx list shows 4 venvs with a missing interpreter (python@3.12 is not installed)",
			"steps":    []map[string]any{{"tool": "run_command", "input": map[string]any{"command": "pipx reinstall-all --python python3.13", "reason": "Rebuild the pipx environments"}}}},
	},
	"clean": {
		{"id": "brew-cache", "category": "Cleanup", "title": "Clear the Homebrew cache", "bytes": 8_900_000_000,
			"why":  "~/Library/Caches/Homebrew holds 8.9 GB of downloads and old versions of packages that are no longer installed.",
			"what": "Homebrew removes old versions and downloads. Nothing installed changes; a reinstall of an old version would download it again.",
			"risk": "normal", "recommended": true,
			"evidence": "brew cleanup --dry-run lists 412 files, 8.9 GB; none belongs to an installed version",
			"steps":    []map[string]any{{"tool": "run_step", "input": map[string]any{"id": "brew-cleanup"}}}},
		{"id": "npm-cache", "category": "Cleanup", "title": "Empty the npm cache", "bytes": 1_300_000_000,
			"why":  "~/.npm/_cacache takes 1.3 GB; npm verifies it on every install and downloads again what it needs.",
			"what": "The next npm install of each package downloads it again. No project changes.",
			"risk": "normal", "recommended": true,
			"evidence": "du -sh ~/.npm/_cacache: 1.3G; npm cache verify: no errors",
			"steps":    []map[string]any{{"tool": "run_step", "input": map[string]any{"id": "npm-cache"}}}},
	},
	"deep": {
		{"id": "xcode-derived", "category": "Deep clean", "title": "Delete Xcode DerivedData", "bytes": 4_100_000_000,
			"why":      "~/Library/Developer/Xcode/DerivedData holds 4.1 GB of build products of 23 projects, the newest from 19 days ago.",
			"what":     "Xcode rebuilds what a project needs the next time it is opened; the first build takes longer. Cannot be undone, nothing is lost.",
			"risk":     "dangerous",
			"evidence": "du -sh DerivedData: 4.1G; no xcodebuild process running; Xcode recreates it",
			"steps":    []map[string]any{{"tool": "run_command", "input": map[string]any{"command": "rm -rf ~/Library/Developer/Xcode/DerivedData/*", "reason": "Delete Xcode build products"}}}},
		{"id": "docker-build-cache", "category": "Docker", "title": "Prune the Docker build cache", "bytes": 6_200_000_000,
			"why":  "docker system df: the build cache takes 6.2 GB, all of it unused for more than 30 days.",
			"what": "Docker removes the cached build layers; the next build of each image takes longer. Images and containers stay.",
			"risk": "normal", "recommended": true,
			"evidence": "docker system df -v: 6.2 GB reclaimable build cache, no build running",
			"steps":    []map[string]any{{"tool": "run_command", "input": map[string]any{"command": "docker builder prune -f", "reason": "Free the Docker build cache"}}}},
	},
	"care": {
		{"id": "agents-left", "category": "AI agents", "title": "End 3 processes closed agent sessions left", "impact": "~1.2 GB RAM",
			"why":  "Two Vite dev servers and a Chromium started by Claude Code sessions that ended yesterday still run and hold 1.2 GB of memory.",
			"what": "omega ends the 3 processes; nothing running in an open session is touched. Start a dev server again when you need it.",
			"risk": "normal", "recommended": true,
			"evidence": "omega agents --json: 3 orphans whose parent session exited 21 h ago",
			"steps":    []map[string]any{{"tool": "omega_cmd", "input": map[string]any{"action": "agents_kill"}}}},
	},
}

// auditTurn plays one turn of an audit stage: look, plan, summarise.
func (c *chat) auditTurn(r *llm.Reply) *llm.Reply {
	acts := demoActions[c.audit]
	switch {
	case c.turn == 1:
		r.Calls = []llm.ToolCall{call(c, "system_info", map[string]any{})}
	case c.turn == 2:
		r.Calls = []llm.ToolCall{step(c, "disk"), step(c, "brew-outdated")}
	case c.turn == 3 && len(acts) > 0:
		r.Text = "Audit findings ready; adding them to the plan."
		r.Calls = []llm.ToolCall{call(c, "plan_actions", map[string]any{"actions": acts})}
	default:
		r.Text = fmt.Sprintf("**Audit of this stage done.** %d actions planned for your review; nothing was changed.", len(acts))
	}
	return r
}
