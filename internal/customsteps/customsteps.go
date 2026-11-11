// Package customsteps loads user-defined steps from steps.d/*.toml, executes
// pre/post lifecycle hooks, and manages filesystem safety snapshots (APFS, Btrfs, Windows Checkpoint).
package customsteps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/lcajigasm/omega/internal/config"
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

// LoadCustomSteps loads step definitions from ~/.config/omega/steps.d/*.toml.
func LoadCustomSteps() ([]steps.Step, error) {
	stepsDir := filepath.Join(config.Dir(), "steps.d")
	if _, err := os.Stat(stepsDir); os.IsNotExist(err) {
		return nil, nil
	}

	files, err := filepath.Glob(filepath.Join(stepsDir, "*.toml"))
	if err != nil || len(files) == 0 {
		return nil, nil
	}

	var loaded []steps.Step

	for _, file := range files {
		var csf CustomStepsFile
		if _, err := toml.DecodeFile(file, &csf); err != nil {
			continue
		}
		for _, def := range csf.Steps {
			if def.ID == "" || def.Command == "" {
				continue
			}
			if def.OS != "" && def.OS != goos {
				continue
			}
			risk := safety.Normal
			switch strings.ToLower(def.Risk) {
			case "read-only", "safe":
				risk = safety.Safe
			case "dangerous":
				risk = safety.Dangerous
			}
			category := def.Category
			if category == "" {
				category = steps.Clean
			}

			s := steps.Step{
				ID:          def.ID,
				Name:        def.Name,
				Category:    category,
				Description: def.Description,
				Command:     def.Command,
				Risk:        risk,
				Requires:    def.Requires,
				OS:          def.OS,
			}
			loaded = append(loaded, s)
		}
	}

	return loaded, nil
}

// RunHook executes a configured lifecycle hook command.
func RunHook(ctx context.Context, hookName string) error {
	stepsDir := filepath.Join(config.Dir(), "steps.d")
	files, _ := filepath.Glob(filepath.Join(stepsDir, "*.toml"))
	for _, file := range files {
		var csf CustomStepsFile
		if _, err := toml.DecodeFile(file, &csf); err == nil {
			if cmd, ok := csf.Hooks[hookName]; ok && cmd != "" {
				res := runner.Run(ctx, cmd, 60*time.Second, nil)
				if res.ExitCode != 0 {
					return fmt.Errorf("hook %s failed: %s", hookName, res.Output)
				}
			}
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
