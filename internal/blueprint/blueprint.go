// Package blueprint exports the system state (installed packages across Homebrew,
// winget, Flatpak, npm, pipx, devtools, and omega settings) and bootstraps a new machine.
package blueprint

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/devtools"
	"github.com/lcajigasm/omega/internal/runner"
)

var goos = runtime.GOOS

// SystemBlueprint holds the complete snapshot of the system configuration and tools.
type SystemBlueprint struct {
	Version      string            `json:"version"`
	ExportDate   time.Time         `json:"export_date"`
	Platform     string            `json:"platform"`
	Hostname     string            `json:"hostname"`
	Homebrew     []string          `json:"homebrew,omitempty"`
	Casks        []string          `json:"casks,omitempty"`
	NpmGlobal    []string          `json:"npm_global,omitempty"`
	PipxPackages []string          `json:"pipx_packages,omitempty"`
	DevTools     []string          `json:"dev_tools,omitempty"`
	Config       *config.Config    `json:"config,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// Export generates a SystemBlueprint and writes it to path.
func Export(ctx context.Context, path string) (*SystemBlueprint, error) {
	hostname, _ := os.Hostname()
	bp := &SystemBlueprint{
		Version:    "1.0",
		ExportDate: time.Now(),
		Platform:   goos,
		Hostname:   hostname,
		Metadata:   make(map[string]string),
	}

	// Homebrew
	if runner.Has("brew") {
		res := runner.Run(ctx, "brew leaves 2>/dev/null", 5*time.Second, nil)
		if res.ExitCode == 0 {
			for _, line := range strings.Split(strings.TrimSpace(res.Output), "\n") {
				if l := strings.TrimSpace(line); l != "" {
					bp.Homebrew = append(bp.Homebrew, l)
				}
			}
		}
		resCasks := runner.Run(ctx, "brew list --cask 2>/dev/null", 5*time.Second, nil)
		if resCasks.ExitCode == 0 {
			for _, line := range strings.Split(strings.TrimSpace(resCasks.Output), "\n") {
				if l := strings.TrimSpace(line); l != "" {
					bp.Casks = append(bp.Casks, l)
				}
			}
		}
	}

	// npm global
	if runner.Has("npm") {
		res := runner.Run(ctx, "npm list -g --depth=0 --parseable 2>/dev/null", 5*time.Second, nil)
		if res.ExitCode == 0 {
			for _, line := range strings.Split(strings.TrimSpace(res.Output), "\n") {
				pkg := strings.TrimSpace(line)
				if pkg != "" && !strings.HasSuffix(pkg, "/lib") && !strings.HasSuffix(pkg, "/npm") {
					bp.NpmGlobal = append(bp.NpmGlobal, filepathBase(pkg))
				}
			}
		}
	}

	// pipx
	if runner.Has("pipx") {
		res := runner.Run(ctx, "pipx list --short 2>/dev/null", 5*time.Second, nil)
		if res.ExitCode == 0 {
			for _, line := range strings.Split(strings.TrimSpace(res.Output), "\n") {
				f := strings.Fields(line)
				if len(f) > 0 {
					bp.PipxPackages = append(bp.PipxPackages, f[0])
				}
			}
		}
	}

	// DevTools inventory
	installed, _ := devtools.Detect()
	for _, t := range installed {
		bp.DevTools = append(bp.DevTools, t.ID)
	}

	// Omega config
	if cfg, err := config.Load(); err == nil {
		bp.Config = cfg
	}

	if path != "" {
		data, err := json.MarshalIndent(bp, "", "  ")
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return nil, err
		}
	}

	return bp, nil
}

// Bootstrap restores tools and configuration from a blueprint file.
func Bootstrap(ctx context.Context, blueprintPath string, logFn func(string)) error {
	data, err := os.ReadFile(blueprintPath)
	if err != nil {
		return err
	}
	var bp SystemBlueprint
	if err := json.Unmarshal(data, &bp); err != nil {
		return fmt.Errorf("parsing blueprint: %w", err)
	}

	if logFn == nil {
		logFn = func(string) {}
	}

	logFn(fmt.Sprintf("Restoring Omega Blueprint from %s (Exported: %s on %s)",
		blueprintPath, bp.ExportDate.Format("2006-01-02"), bp.Platform))

	// Restore Config
	if bp.Config != nil {
		logFn("Restoring Omega configuration...")
		_ = bp.Config.Save()
	}

	// Install Homebrew packages
	if runner.Has("brew") && len(bp.Homebrew) > 0 {
		logFn(fmt.Sprintf("Installing %d Homebrew formulae...", len(bp.Homebrew)))
		cmd := "brew install " + strings.Join(bp.Homebrew, " ")
		runner.Run(ctx, cmd, 30*time.Minute, logFn)
	}
	if runner.Has("brew") && len(bp.Casks) > 0 {
		logFn(fmt.Sprintf("Installing %d Homebrew casks...", len(bp.Casks)))
		cmd := "brew install --cask " + strings.Join(bp.Casks, " ")
		runner.Run(ctx, cmd, 30*time.Minute, logFn)
	}

	// Install npm packages
	if runner.Has("npm") && len(bp.NpmGlobal) > 0 {
		logFn(fmt.Sprintf("Installing %d global npm packages...", len(bp.NpmGlobal)))
		cmd := "npm install -g " + strings.Join(bp.NpmGlobal, " ")
		runner.Run(ctx, cmd, 15*time.Minute, logFn)
	}

	// Install pipx packages
	if runner.Has("pipx") && len(bp.PipxPackages) > 0 {
		for _, pkg := range bp.PipxPackages {
			logFn(fmt.Sprintf("Installing pipx package: %s", pkg))
			runner.Run(ctx, "pipx install "+pkg, 5*time.Minute, logFn)
		}
	}

	logFn("Bootstrap complete!")
	return nil
}

func filepathBase(p string) string {
	parts := strings.Split(p, "/")
	if len(parts) == 0 {
		return p
	}
	return parts[len(parts)-1]
}
