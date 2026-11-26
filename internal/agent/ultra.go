package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/deepclean"
	"github.com/lcajigasm/omega/internal/gitops"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/safety"
)

// omega ultra: the whole of omega, attended, on a stronger model, digging
// deeper and asking before every change. Its sessions get four tools the
// others do not: deep_scan, deep_inspect and deep_clean for what the
// catalogue steps never reach (the worktrees agents leave, the files their
// runs write, Docker object by object), and omega_cmd for every
// deterministic omega command (agents, assistants, disk limits, build
// folders, duplicates, git, privacy, audit, network, skills).

// UltraSuffix tells the model how an ultra run works. It is added to every
// stage's prompt.
const UltraSuffix = "\n\nULTRA MODE (omega ultra). The user asked for the deepest, most thorough pass omega can make and is present to " +
	"confirm every change. Be inquisitive and aggressive in looking, conservative in concluding:\n" +
	"- Look everywhere this stage covers, not only where the catalogue steps look: measure (disk_usage, deep_scan, omega_cmd) " +
	"before and after, follow every folder over 500 MB down to what fills it, and read the logs and configuration behind every " +
	"warning. Use omega_cmd for what omega already does deterministically instead of improvising du, find or rm.\n" +
	"- Validate before you propose. Every option you offer must rest on evidence you gathered in this session: the command or " +
	"tool output that shows it is safe (deep_inspect for a finding, git status and the pull request state for a worktree, " +
	"what a cache is and who recreates it). Put that evidence, in one line, in the option's evidence field: omega refuses options " +
	"that change something without it. If you could not verify something, investigate more or leave it out and say why.\n" +
	"- Cross-check before deleting: confirm nothing running uses it, that what goes can be recreated (a registry, a lockfile, a " +
	"remote branch), and that the size is real. Prefer the reversible way (the Trash, a rescue copy) when there is one.\n" +
	"- Never improvise a bulk deletion (rm -rf of a parent folder, docker image prune -a, git clean -fdx): use the tools that " +
	"remove one checked thing at a time. Images built on this machine are not pruned with the others.\n" +
	"- Rank what you offer by space freed and risk, recommend only what you verified, and never preselect Docker volumes, " +
	"worktrees with work to rescue or anything worked on in the last two days.\n" +
	"- Every change is put to the user (approval cards stay on, whatever was approved earlier). Close each stage with what " +
	"was freed (measured, not estimated), what was verified and left, and why."

// UltraDeepTask is the deep-clean stage of omega ultra.
const UltraDeepTask = "Deep clean this machine's disk, which the ordinary cleaning stages never reach. Start with system_info and deep_scan. " +
	"Then, for each category, worst offenders first: " +
	"(1) AI coding agents: the git worktrees agents (Claude Code, Codex, Cursor) and the user created, and the run files agents " +
	"wrote next to the projects. Each worktree comes with its remote state (remote): whether its commits are on the server (checked with " +
	"git ls-remote, not only the last fetch), whether its branch was deleted there, and each uncommitted file compared by content " +
	"with the remote branches (files_on_remote, files_only_here). Start every worktree option's label with that state in a few " +
	"words (\"all on the remote\", \"2 commits not pushed\", \"3 files only here\") so the user can choose by it, and group the " +
	"options: everything on the remote first, then what has something only here. For every worktree over 100 MB or marked review, " +
	"call deep_inspect: read every file only here, the last commits and the pull request state; a worktree whose PR is merged or " +
	"closed and whose content is all on the remote is done work. " +
	"(2) Docker, object by object (deep_scan lists them): images from a registry, dangling images and stopped containers that " +
	"changed nothing are safe; images built here and containers with their own changes need the user's word; volumes are never " +
	"recommended. (3) Build folders: omega_cmd artifacts_scan with days 14 (node_modules, target, .venv, dist… of projects idle " +
	"two weeks). (4) git: omega_cmd git_status and git_clean_preview (merged branches), then git_gc. (5) Disk limits and old " +
	"versions: omega_cmd disk_limits and limits_enforce_preview. (6) Large and duplicate files in the home folder: omega_cmd large " +
	"and duplicates, only to report: never offer to delete the user's documents, media or downloads. " +
	"Offer everything with propose_actions (multiple: true), one option per finding or per command, grouped by category, " +
	"each with its size and evidence. Carry out what is picked with deep_clean (findings) and omega_cmd (commands), measure the " +
	"free space again, and summarise what was freed, what was rescued and where, and what was left and why."

// UltraCareTask is the stage of omega ultra that runs omega's own
// reviews outside disk space.
const UltraCareTask = "Go through every review omega has outside the maintenance stages, with omega_cmd, and act on what you verify: " +
	"(1) AI agents left running: agents_list; offer to end what closed sessions left (dev servers, watchers, browsers) and sessions " +
	"idle for hours. (2) Configuration assistants: assist; for every installed tool, the pending checks (Claude Code, Codex, Cursor, " +
	"Docker, Homebrew, Xcode, Ollama…): disk limits, deny rules that keep agents out of keys and .env files, MCP registration. " +
	"(3) Security: audit_ssh and audit_secrets (keys with loose permissions, tokens left in shell histories: say where, never " +
	"print a secret). (4) Privacy: privacy (telemetry settings, browser caches). (5) Network: net_audit (hosts file, proxies). " +
	"(6) Agent skills: skills_audit (broken links, bad SKILL.md) and skills_list. (7) What is left from earlier runs: pending. " +
	"Explain each finding in a sentence, verify it, and offer the fixes with propose_actions (multiple: true), each with its " +
	"evidence. Apply only what is picked, with omega_cmd."

// ultraModePrompt is added to the run mode of an ultra session.
const ultraModePrompt = " ULTRA: a person confirms every change; every option that changes something must carry its evidence."

func ultraToolSpecs() []llm.ToolSpec {
	return []llm.ToolSpec{
		{Name: "deep_scan", Description: "omega ultra: find what the cleaning steps never reach, without changing anything: git worktrees " +
			"(the ones AI agents leave and the user's, each with how much of it is on the remote: commits checked with the server, " +
			"uncommitted files compared by content), run files agents wrote next to the projects, and Docker images, stopped containers " +
			"and volumes one by one. Each finding has an id, a size, a verdict (safe: nothing is lost; review: something would be lost " +
			"or must be rescued first, or it was used in the last two days; keep: never removed) and the reasons. JSON.",
			Properties: map[string]any{"docker": map[string]any{"type": "boolean", "description": "include Docker (default true)"}}},
		{Name: "deep_inspect", Description: "omega ultra: the evidence behind one deep_scan finding: every file git reports, the diff, the " +
			"last commits, the branches that hold them and the pull request state (gh); or a Docker object's details. Read-only. Call it " +
			"before offering any worktree, container or image built here.",
			Properties: map[string]any{"id": str("finding id from deep_scan")}, Required: []string{"id"}},
		{Name: "deep_clean", Description: "omega ultra: remove deep_scan findings the user picked, one at a time. Each one is checked again " +
			"just before (refused if it changed), put to the user for approval, and a worktree's uncommitted work is saved first under " +
			"omega's rescued folder. Agent files go to the Trash. Findings marked keep are never removed.",
			Properties: map[string]any{"ids": strList("finding ids from deep_scan")}, Required: []string{"ids"}},
		{Name: "omega_cmd", Description: "omega ultra: run one of omega's own commands. Read-only: agents_list, assist, disk_limits, " +
			"limits_enforce_preview, artifacts_scan, large, duplicates, git_status, git_clean_preview, privacy, audit_ssh, audit_secrets, " +
			"net_audit, skills_list, skills_audit, pending. Changes (each one put to the user): agents_kill (what closed sessions left), " +
			"agents_idle (sessions idle over 2h), assist_apply (check, value optional), limits_set (check, value), limits_apply, " +
			"limits_enforce, artifacts_delete (days), git_clean (merged branches), git_gc, privacy_clean (browser caches), net_flush, " +
			"skills_sync.",
			Properties: map[string]any{
				"action": map[string]any{"type": "string", "enum": ultraActionNames()},
				"check":  str("for assist_apply and limits_set: the check id"),
				"value":  str("for assist_apply and limits_set: the value (14, 10GB, off)"),
				"days":   map[string]any{"type": "integer", "description": "for artifacts_scan and artifacts_delete: idle days (default 14)"},
				"folder": str("for large and duplicates: the folder (default the home folder)"),
			}, Required: []string{"action"}},
	}
}

// OmegaRun runs this omega with arguments, no terminal, and returns what it
// printed. The CLI sets it.
var OmegaRun func(ctx context.Context, args ...string) (string, error)

// ultraAction is one omega command omega_cmd runs.
type ultraAction struct {
	args func(in map[string]any) ([]string, error)
	// changes: put to the user every time.
	changes bool
}

var plainWord = regexp.MustCompile(`^[A-Za-z0-9._:/~+@-]{1,200}$`)

func word(in map[string]any, key string, required bool) (string, error) {
	v := strings.TrimSpace(getString(in, key))
	if v == "" {
		if required {
			return "", fmt.Errorf("%s is required", key)
		}
		return "", nil
	}
	if !plainWord.MatchString(v) || strings.HasPrefix(v, "-") {
		return "", fmt.Errorf("%s: %q is not a plain value", key, v)
	}
	return v, nil
}

func days(in map[string]any) string {
	switch v := in["days"].(type) {
	case float64:
		if v >= 1 {
			return strconv.Itoa(int(v))
		}
	case string:
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			return v
		}
	}
	return "14"
}

func fixed(args ...string) func(map[string]any) ([]string, error) {
	return func(map[string]any) ([]string, error) { return args, nil }
}

var ultraActions = map[string]ultraAction{
	"agents_list":            {args: fixed("agents", "--json")},
	"assist":                 {args: fixed("assist", "--json")},
	"disk_limits":            {args: fixed("disk", "limits", "--json")},
	"limits_enforce_preview": {args: fixed("disk", "limits", "enforce", "--dry-run")},
	"artifacts_scan": {args: func(in map[string]any) ([]string, error) {
		return []string{"disk", "artifacts", days(in), "--dry-run", "--json"}, nil
	}},
	"large": {args: func(in map[string]any) ([]string, error) {
		d, err := word(in, "folder", false)
		if err != nil || d == "" {
			return []string{"disk", "large", "--min", "500MB", "--limit", "40"}, err
		}
		return []string{"disk", "large", d, "--min", "500MB", "--limit", "40"}, nil
	}},
	"duplicates": {args: func(in map[string]any) ([]string, error) {
		d, err := word(in, "folder", false)
		if err != nil || d == "" {
			return []string{"disk", "duplicates", "--min", "20MB", "--limit", "30", "--json"}, err
		}
		return []string{"disk", "duplicates", d, "--min", "20MB", "--limit", "30", "--json"}, nil
	}},
	"git_status":        {args: fixed("git", "status", "--json")},
	"git_clean_preview": {args: fixed("git", "clean", "--dry-run")},
	"privacy":           {args: fixed("privacy", "--json")},
	"audit_ssh":         {args: fixed("audit", "ssh", "--json")},
	"audit_secrets":     {args: fixed("audit", "secrets", "--json", "--no-fail")},
	"net_audit":         {args: fixed("net", "audit", "--json")},
	"skills_list":       {args: fixed("skills", "list", "--json")},
	"skills_audit":      {args: fixed("skills", "audit")},
	"pending":           {args: fixed("pending")},

	"agents_kill": {args: fixed("agents", "kill", "--yes"), changes: true},
	"agents_idle": {args: fixed("agents", "idle", "--yes"), changes: true},
	"assist_apply": {changes: true, args: func(in map[string]any) ([]string, error) {
		c, err := word(in, "check", true)
		if err != nil {
			return nil, err
		}
		v, err := word(in, "value", false)
		if err != nil {
			return nil, err
		}
		if v == "now" {
			return nil, fmt.Errorf("a cleanup that deletes files goes through limits_enforce or deep_clean")
		}
		a := []string{"assist", "apply", c}
		if v != "" {
			a = append(a, v)
		}
		return append(a, "--yes"), nil
	}},
	"limits_set": {changes: true, args: func(in map[string]any) ([]string, error) {
		c, err := word(in, "check", true)
		if err != nil {
			return nil, err
		}
		v, err := word(in, "value", true)
		if err != nil {
			return nil, err
		}
		return []string{"disk", "limits", "set", c, v}, nil
	}},
	"limits_apply":   {args: fixed("disk", "limits", "apply", "--yes"), changes: true},
	"limits_enforce": {args: fixed("disk", "limits", "enforce", "--yes"), changes: true},
	"artifacts_delete": {changes: true, args: func(in map[string]any) ([]string, error) {
		return []string{"disk", "artifacts", days(in), "--yes"}, nil
	}},
	"git_clean":     {args: fixed("git", "clean", "--yes"), changes: true},
	"git_gc":        {args: fixed("git", "gc"), changes: true},
	"privacy_clean": {args: fixed("privacy", "--clean", "--yes"), changes: true},
	"net_flush":     {args: fixed("net", "flush"), changes: true},
	"skills_sync":   {args: fixed("skills", "sync"), changes: true},
}

func ultraActionNames() []string {
	names := make([]string, 0, len(ultraActions))
	for n := range ultraActions {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// The two stages only omega ultra has.
const (
	TaskUltraDeep = "ultra-deep"
	TaskUltraCare = "ultra-care"
)

func init() {
	TaskPrompts[TaskUltraDeep] = UltraDeepTask
	TaskPrompts[TaskUltraCare] = UltraCareTask
	handlers["deep_scan"] = toolDeepScan
	handlers["deep_inspect"] = toolDeepInspect
	handlers["deep_clean"] = toolDeepClean
	handlers["omega_cmd"] = toolOmegaCmd
}

// errNotUltra answers an ultra tool outside an ultra run.
var errNotUltra = fmt.Errorf("this tool exists only in omega ultra")

// Seams the tests replace.
var (
	deepScan    = deepclean.Scan
	deepInspect = deepclean.Inspect
	deepApply   = deepclean.Apply
	deepRoots   = func() ([]string, []string) {
		roots := gitops.DefaultRoots()
		return roots, gitops.FindRepos(roots)
	}
)

func toolDeepScan(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	if !a.opts.Ultra {
		return "", errNotUltra
	}
	start := time.Now()
	docker := true
	if v, ok := in["docker"].(bool); ok {
		docker = v
	}
	roots, repos := deepRoots()
	r := deepScan(ctx, deepclean.Options{Roots: roots, Repos: repos, Docker: docker})
	a.mu.Lock()
	a.deep = &r
	a.mu.Unlock()
	safe, review, keep := r.Totals()
	out, err := json.MarshalIndent(struct {
		Roots       []string `json:"roots"`
		SafeBytes   int64    `json:"safe_bytes"`
		ReviewBytes int64    `json:"review_bytes"`
		KeptBytes   int64    `json:"kept_bytes"`
		deepclean.Report
	}{roots, safe, review, keep, r}, "", " ")
	a.quiet("deep_scan", "omega ultra deep scan", start, string(out), err)
	return string(out), err
}

// deepFinding is a finding of the last deep_scan.
func (a *Agent) deepFinding(id string) (deepclean.Finding, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.deep == nil {
		return deepclean.Finding{}, false
	}
	return a.deep.Find(id)
}

func toolDeepInspect(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	if !a.opts.Ultra {
		return "", errNotUltra
	}
	f, ok := a.deepFinding(getString(in, "id"))
	if !ok {
		return "", fmt.Errorf("no finding %q: run deep_scan first and use its ids", getString(in, "id"))
	}
	start := time.Now()
	out := deepInspect(ctx, f)
	a.quiet("deep_inspect", f.Title, start, out, nil)
	return out, nil
}

func toolDeepClean(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	if !a.opts.Ultra {
		return "", errNotUltra
	}
	ids := getStrings(in, "ids")
	if len(ids) == 0 {
		return "", fmt.Errorf("ids is empty")
	}
	var b strings.Builder
	var freed int64
	for _, id := range ids {
		f, ok := a.deepFinding(id)
		if !ok {
			fmt.Fprintf(&b, "- %s: not a finding of the last deep_scan; nothing done\n", id)
			continue
		}
		if f.Verdict == deepclean.Keep {
			fmt.Fprintf(&b, "- %s (%s): kept, never removed: %s\n", id, f.Title, strings.Join(f.Reasons, "; "))
			continue
		}
		why := fmt.Sprintf("%s · %s · %s: %s", f.Title, humanBytes(f.Size), f.Verdict, strings.Join(f.Reasons, "; "))
		if f.Remote != nil {
			why = fmt.Sprintf("%s · %s · remote: %s · %s: %s", f.Title, humanBytes(f.Size), f.Remote.Label(), f.Verdict, strings.Join(f.Reasons, "; "))
		}
		if len(f.Rescue) > 0 {
			why += " · saved first: " + strings.Join(f.Rescue, "; ")
		}
		if ok, reason := a.authorize(ctx, "deep_clean", f.Action, why, safety.Dangerous, true); !ok {
			fmt.Fprintf(&b, "- %s (%s): not done: %s\n", id, f.Title, reason)
			continue
		}
		a.forget()
		start := time.Now()
		res, err := deepApply(ctx, f)
		a.quiet("deep_clean", f.Action, start, "", err)
		if err != nil {
			fmt.Fprintf(&b, "- %s (%s): not removed: %v\n", id, f.Title, err)
			continue
		}
		freed += res.Freed
		fmt.Fprintf(&b, "- %s (%s): removed, %s freed", id, f.Title, humanBytes(res.Freed))
		if res.Rescued != "" {
			fmt.Fprintf(&b, "; its work was saved in %s", res.Rescued)
		}
		if res.Note != "" {
			fmt.Fprintf(&b, "; %s", res.Note)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Total freed: %s. Measure the free space again before reporting it.", humanBytes(freed))
	return b.String(), nil
}

func toolOmegaCmd(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	if !a.opts.Ultra {
		return "", errNotUltra
	}
	if OmegaRun == nil {
		return "", fmt.Errorf("omega's commands are not available in this session")
	}
	name := getString(in, "action")
	act, ok := ultraActions[name]
	if !ok {
		return "", fmt.Errorf("unknown action %q; one of %s", name, strings.Join(ultraActionNames(), ", "))
	}
	args, err := act.args(in)
	if err != nil {
		return "", err
	}
	line := "omega " + strings.Join(args, " ")
	if act.changes {
		if ok, reason := a.authorize(ctx, "omega_cmd", line, "omega ultra · "+name, safety.Dangerous, true); !ok {
			return "", explained(fmt.Errorf("%s", reason))
		}
		a.forget()
	} else if out, ok := a.cached("omega_cmd", line, safety.Safe); ok {
		return out, nil
	}
	start := time.Now()
	out, err := OmegaRun(ctx, args...)
	a.quiet("omega_cmd", line, start, out, err)
	if err != nil {
		return "", err
	}
	if !act.changes {
		a.remember("omega_cmd", line, out)
	}
	return out, nil
}

// ultraEvidence checks, in an ultra run, that every option that changes
// something says what verified it. It returns why the card is refused, or
// "" when it may be shown.
func ultraEvidence(opts []Choice, evidence map[string]string) string {
	var missing []string
	for _, c := range opts {
		if c.Risk == "read-only" || noChange.MatchString(c.ID) || noChange.MatchString(c.Label) {
			continue
		}
		if len(strings.TrimSpace(evidence[c.ID])) < 12 {
			missing = append(missing, c.ID)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return "omega ultra refused this card: these options change something and say nothing about what verified them: " +
		strings.Join(missing, ", ") + ". Investigate each one (deep_inspect, the command output that proves it is safe and " +
		"what it frees), then call propose_actions again with an evidence line on every such option. Leave out what you cannot verify."
}

func humanBytes(n int64) string {
	switch {
	case n >= 1e9:
		return fmt.Sprintf("%.1f GB", float64(n)/1e9)
	case n >= 1e6:
		return fmt.Sprintf("%.0f MB", float64(n)/1e6)
	case n >= 1e3:
		return fmt.Sprintf("%.0f kB", float64(n)/1e3)
	}
	return fmt.Sprintf("%d B", n)
}
