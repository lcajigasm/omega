// Package customsteps loads user-defined steps from steps.d/*.toml, executes
// pre/post lifecycle hooks, and manages filesystem safety snapshots (APFS, Btrfs, Windows Checkpoint).
package customsteps

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/safety"
	"github.com/lcajigasm/omega/internal/steps"
)

var goos = runtime.GOOS

// CustomStepDef matches the TOML schema for user-defined steps.
type CustomStepDef struct {
	ID          string `toml:"id"`
	Name        string `toml:"name"`
	Category    string `toml:"category"` // "update", "clean", "check", "backend"
	Description string `toml:"description"`
	Command     string `toml:"command"`
	Risk        string `toml:"risk"` // "read-only", "normal", "dangerous"
	Requires    string `toml:"requires"`
	OS          string `toml:"os"` // "darwin", "linux", "windows", or empty
}

// CustomStepsFile represents a .toml file in steps.d.
type CustomStepsFile struct {
	Steps []CustomStepDef   `toml:"steps"`
	Hooks map[string]string `toml:"hooks"`
}

// files are the steps.d/*.toml files, in name order.
func files() []string {
	list, _ := filepath.Glob(filepath.Join(config.Dir(), "steps.d", "*.toml"))
	return list
}

// Load reads the step definitions in ~/.config/omega/steps.d/*.toml. It
// returns the steps that are usable and one line for everything that is not:
// a file that does not parse, a step with no id or command, a category or a
// risk omega does not know. A step left out without a word would look
// installed to its author and never run.
func Load() (loaded []steps.Step, problems []string) {
	for _, file := range files() {
		if why := untrusted(file); why != "" {
			problems = append(problems, i18n.T("custom.untrusted", file, why))
			continue
		}
		var csf CustomStepsFile
		if _, err := toml.DecodeFile(file, &csf); err != nil {
			problems = append(problems, i18n.T("custom.parse", file, err))
			continue
		}
		for name, cmd := range csf.Hooks {
			if safety.Classify(cmd) == safety.Blocked {
				problems = append(problems, i18n.T("custom.hook_blocked", file, name))
			}
		}
		for _, def := range csf.Steps {
			if def.ID == "" || def.Command == "" {
				problems = append(problems, i18n.T("custom.incomplete", file))
				continue
			}
			if def.OS != "" && def.OS != goos {
				continue
			}
			// The safety policy has no exception for a file in steps.d:
			// what omega never runs for the model it does not run from
			// here, and a step of its own keeps its command and its risk.
			if safety.Classify(def.Command) == safety.Blocked {
				problems = append(problems, i18n.T("custom.blocked", file, def.ID))
				continue
			}
			if steps.Builtin(def.ID) {
				problems = append(problems, i18n.T("custom.builtin", file, def.ID))
				continue
			}
			var risk safety.Risk
			switch strings.ToLower(def.Risk) {
			case "read-only", "safe":
				risk = safety.Safe
			case "", "normal":
				risk = safety.Normal
			case "dangerous":
				risk = safety.Dangerous
			default:
				problems = append(problems, i18n.T("custom.bad_risk", file, def.ID, def.Risk))
				continue
			}
			// A step is at least as risky as its command: "read-only" on an
			// rm -rf would run it in --dry-run, in read-only mode and in
			// analyze and health.
			if r := safety.Classify(def.Command); r > risk {
				risk = r
			}
			category := def.Category
			if category == "" {
				category = steps.Clean
			}
			if !slices.Contains(steps.Categories, category) {
				problems = append(problems, i18n.T("custom.bad_category", file, def.ID, def.Category, strings.Join(steps.Categories, ", ")))
				continue
			}
			loaded = append(loaded, steps.Step{
				ID:          def.ID,
				Name:        def.Name,
				Category:    category,
				Description: def.Description,
				Command:     def.Command,
				Risk:        risk,
				Requires:    def.Requires,
				OS:          def.OS,
			})
		}
	}
	return loaded, problems
}

// untrusted says why a steps.d file is not read: the file or its folder can
// be written by any user of this machine, or belongs to another one. Its
// commands run as you, some with sudo and some with nobody watching, so it
// has to be yours alone. "" means the file is fine.
func untrusted(file string) string {
	for _, path := range []string{file, filepath.Dir(file)} {
		info, err := os.Stat(path)
		if err != nil {
			return err.Error()
		}
		if goos == "windows" {
			continue
		}
		if info.Mode().Perm()&0o002 != 0 {
			return i18n.T("custom.world_writable", path)
		}
		if uid, ok := owner(info); ok && uid != 0 && int(uid) != os.Getuid() {
			return i18n.T("custom.not_yours", path)
		}
	}
	return ""
}

// Hook is one command a steps.d file asks to run around a task.
type Hook struct {
	File    string
	Command string
}

// Hooks lists the commands configured for a hook (pre_update, post_clean,
// on_error…), in file order. A file that does not parse has none; Load
// reports it.
func Hooks(name string) []Hook {
	var out []Hook
	for _, file := range files() {
		if untrusted(file) != "" {
			continue
		}
		var csf CustomStepsFile
		if _, err := toml.DecodeFile(file, &csf); err != nil {
			continue
		}
		// A hook the safety policy blocks is reported by Load, not run.
		if cmd := csf.Hooks[name]; cmd != "" && safety.Classify(cmd) != safety.Blocked {
			out = append(out, Hook{File: file, Command: cmd})
		}
	}
	return out
}

// run is a seam the tests replace.
var run = runner.Run

// RunHook runs every command configured for a hook and stops at the first
// one that fails.
func RunHook(ctx context.Context, name string) error {
	for _, h := range Hooks(name) {
		if res := run(ctx, h.Command, 60*time.Second, nil); res.ExitCode != 0 {
			msg := i18n.T("custom.hook_failed", name, h.File, res.ExitCode)
			if out := strings.TrimSpace(res.Output); out != "" {
				msg += ": " + out
			}
			return fmt.Errorf("%s", msg)
		}
	}
	return nil
}

// has is a seam the tests replace.
var has = runner.Has

// CreateSnapshot creates a filesystem restore point before major maintenance.
func CreateSnapshot(ctx context.Context, desc string) (string, error) {
	if desc == "" {
		desc = "omega-pre-maintenance"
	}
	switch goos {
	case "darwin":
		res := run(ctx, "tmutil snapshot", 30*time.Second, nil)
		if res.ExitCode != 0 {
			return "", fmt.Errorf("tmutil snapshot failed: %s", res.Output)
		}
		return "APFS local snapshot created successfully: " + strings.TrimSpace(res.Output), nil
	case "windows":
		script := fmt.Sprintf(`Checkpoint-Computer -Description '%s' -RestorePointType "MODIFY_SETTINGS"`, strings.ReplaceAll(desc, "'", "''"))
		res := run(ctx, script, 30*time.Second, nil)
		if res.ExitCode != 0 {
			return "", fmt.Errorf("windows checkpoint failed: %s", res.Output)
		}
		return "Windows Restore Point created successfully", nil
	default:
		if has("snapper") {
			// -n: without omega's sudo bridge, fail at once instead of
			// waiting for a password nobody can type.
			res := run(ctx, "sudo -n snapper create --description "+runner.ShellQuote(desc), 15*time.Second, nil)
			if res.ExitCode == 0 {
				return "Btrfs snapper snapshot created successfully", nil
			}
		}
		return "Filesystem snapshots not supported or snapper not configured on this Linux installation", nil
	}
}

// Errors of ListSnapshots.
var (
	// ErrNoSnapshots: the system keeps snapshots, and there are none.
	ErrNoSnapshots = errors.New("no snapshots")
	// ErrNeedsSudo: listing them needs the administrator password, and
	// omega does not wait for one at a prompt nobody may see.
	ErrNeedsSudo = errors.New("listing the snapshots needs sudo")
	// ErrUnsupported: this system has no snapshots omega can list.
	ErrUnsupported = errors.New("no snapshot support")
)

// Snapshots are the restore points of this system.
type Snapshots struct {
	Kind    string   // "apfs", "snapper" or "windows"
	List    []string // one line per snapshot, oldest first as the system lists them
	Restore string   // how to restore one, in the user's language
}

// ListSnapshots lists the restore points. With none, List is empty and the
// error is ErrNoSnapshots; ErrNeedsSudo and ErrUnsupported say why there is
// no list at all. Another error is a listing that failed.
func ListSnapshots(ctx context.Context) (Snapshots, error) {
	switch goos {
	case "darwin":
		snap := Snapshots{Kind: "apfs", Restore: i18n.T("snapshots.restore_apfs")}
		res := run(ctx, "tmutil listlocalsnapshots /", 10*time.Second, nil)
		if res.ExitCode != 0 {
			return snap, fmt.Errorf("tmutil listlocalsnapshots: %s", strings.TrimSpace(res.Output))
		}
		for _, l := range strings.Split(res.Output, "\n") {
			l = strings.TrimSpace(l)
			// The first line is "Snapshots for disk /:".
			if l == "" || strings.HasSuffix(l, ":") || strings.HasPrefix(l, "[omega") {
				continue
			}
			snap.List = append(snap.List, l)
		}
		return done(snap)
	case "windows":
		snap := Snapshots{Kind: "windows", Restore: i18n.T("snapshots.restore_windows")}
		res := run(ctx, "Get-ComputerRestorePoint | ForEach-Object { \"$($_.SequenceNumber)  $($_.ConvertToDateTime($_.CreationTime))  $($_.Description)\" }", 15*time.Second, nil)
		if res.ExitCode != 0 {
			return snap, fmt.Errorf("Get-ComputerRestorePoint: %s", strings.TrimSpace(res.Output))
		}
		for _, l := range strings.Split(res.Output, "\n") {
			if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "[omega") {
				snap.List = append(snap.List, l)
			}
		}
		return done(snap)
	default:
		snap := Snapshots{Kind: "snapper", Restore: i18n.T("snapshots.restore_snapper")}
		if !has("snapper") {
			return snap, ErrUnsupported
		}
		// snapper lets the users in ALLOW_USERS list without sudo; for the
		// others sudo -n fails at once when it needs a password.
		res := run(ctx, "snapper list 2>&1", 10*time.Second, nil)
		if res.ExitCode != 0 {
			res = run(ctx, "sudo -n snapper list 2>&1", 10*time.Second, nil)
		}
		if res.ExitCode != 0 {
			out := strings.ToLower(res.Output)
			if strings.Contains(out, "password is required") || strings.Contains(out, "a terminal is required") ||
				strings.Contains(out, "sudo:") || strings.Contains(out, "no permissions") {
				return snap, ErrNeedsSudo
			}
			return snap, fmt.Errorf("snapper list: %s", strings.TrimSpace(res.Output))
		}
		snap.List = snapperRows(res.Output)
		return done(snap)
	}
}

func done(s Snapshots) (Snapshots, error) {
	if len(s.List) == 0 {
		return s, ErrNoSnapshots
	}
	return s, nil
}

// snapperRows reads the table of snapper list: the rows after the rule
// under the header, without snapshot 0 (the current system).
func snapperRows(out string) []string {
	var rows []string
	body := false
	for _, l := range strings.Split(out, "\n") {
		t := strings.TrimSpace(l)
		if !body {
			body = strings.Contains(t, "-+-") || strings.Contains(t, "─┼─")
			continue
		}
		f := strings.FieldsFunc(t, func(r rune) bool { return r == '|' || r == '│' })
		if len(f) == 0 {
			continue
		}
		id := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(f[0]), "*-+"))
		if id == "" || id == "0" || strings.Trim(id, "0123456789") != "" {
			continue
		}
		rows = append(rows, t)
	}
	return rows
}

// Rollback lists the restore points and says how to restore one. It says so
// when there are none, and when listing them needs sudo.
func Rollback(ctx context.Context) (string, error) {
	snap, err := ListSnapshots(ctx)
	switch {
	case errors.Is(err, ErrNoSnapshots):
		return i18n.T("snapshots.none"), nil
	case errors.Is(err, ErrUnsupported):
		return i18n.T("snapshots.unsupported"), nil
	case errors.Is(err, ErrNeedsSudo):
		return "", fmt.Errorf("%s", i18n.T("snapshots.needs_sudo"))
	case err != nil:
		return "", err
	}
	var b strings.Builder
	b.WriteString(i18n.T("snapshots.available", len(snap.List)))
	b.WriteString("\n")
	for _, l := range snap.List {
		b.WriteString("  " + l + "\n")
	}
	b.WriteString(snap.Restore)
	return b.String(), nil
}
