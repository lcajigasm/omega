// Package customsteps loads user-defined steps from steps.d/*.toml, executes
// pre/post lifecycle hooks, and manages filesystem safety snapshots (APFS, Btrfs, Windows Checkpoint).
package customsteps

import (
	"context"
	"fmt"
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
		var csf CustomStepsFile
		if _, err := toml.DecodeFile(file, &csf); err != nil {
			problems = append(problems, i18n.T("custom.parse", file, err))
			continue
		}
		for _, def := range csf.Steps {
			if def.ID == "" || def.Command == "" {
				problems = append(problems, i18n.T("custom.incomplete", file))
				continue
			}
			if def.OS != "" && def.OS != goos {
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
		var csf CustomStepsFile
		if _, err := toml.DecodeFile(file, &csf); err != nil {
			continue
		}
		if cmd := csf.Hooks[name]; cmd != "" {
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

// CreateSnapshot creates a filesystem restore point before major maintenance.
func CreateSnapshot(ctx context.Context, desc string) (string, error) {
	if desc == "" {
		desc = "omega-pre-maintenance"
	}
	switch goos {
	case "darwin":
		res := runner.Run(ctx, "tmutil snapshot", 30*time.Second, nil)
		if res.ExitCode != 0 {
			return "", fmt.Errorf("tmutil snapshot failed: %s", res.Output)
		}
		return "APFS local snapshot created successfully: " + strings.TrimSpace(res.Output), nil
	case "windows":
		script := fmt.Sprintf(`Checkpoint-Computer -Description "%s" -RestorePointType "MODIFY_SETTINGS"`, desc)
		res := runner.Run(ctx, script, 30*time.Second, nil)
		if res.ExitCode != 0 {
			return "", fmt.Errorf("windows checkpoint failed: %s", res.Output)
		}
		return "Windows Restore Point created successfully", nil
	default:
		if runner.Has("snapper") {
			cmd := fmt.Sprintf(`sudo snapper create --description "%s"`, desc)
			res := runner.Run(ctx, cmd, 15*time.Second, nil)
			if res.ExitCode == 0 {
				return "Btrfs snapper snapshot created successfully", nil
			}
		}
		return "Filesystem snapshots not supported or snapper not configured on this Linux installation", nil
	}
}

// Rollback guides the user or executes restore from the latest snapshot.
func Rollback(ctx context.Context) (string, error) {
	switch goos {
	case "darwin":
		// List available APFS snapshots
		res := runner.Run(ctx, "tmutil listlocalsnapshots /", 5*time.Second, nil)
		return "Available APFS snapshots:\n" + res.Output + "\nTo restore, boot into macOS Recovery and select 'Restore From Time Machine'.", nil
	case "windows":
		res := runner.Run(ctx, "Get-ComputerRestorePoint | Select-Object SequenceNumber, Description, CreationTime | Out-String", 5*time.Second, nil)
		return "Available Windows Restore Points:\n" + res.Output + "\nRun 'rstrui.exe' to launch the System Restore wizard.", nil
	default:
		if runner.Has("snapper") {
			res := runner.Run(ctx, "sudo snapper list", 5*time.Second, nil)
			return "Available snapper snapshots:\n" + res.Output + "\nUse 'snapper undochange <id>..<id>' to rollback changes.", nil
		}
		return "No snapshot rollbacks available on this system.", nil
	}
}
