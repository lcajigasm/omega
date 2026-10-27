package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/steps"
	"github.com/lcajigasm/omega/internal/ui"
)

type dockerDF struct {
	Type        string `json:"Type"`
	TotalCount  string `json:"TotalCount"`
	Active      string `json:"Active"`
	Size        string `json:"Size"`
	Reclaimable string `json:"Reclaimable"`
}

func dockerUsage() ([]dockerDF, error) {
	out, err := exec.Command("docker", "system", "df", "--format", "{{json .}}").Output()
	if err != nil {
		return nil, err
	}
	var rows []dockerDF
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		var r dockerDF
		if json.Unmarshal([]byte(line), &r) == nil {
			rows = append(rows, r)
		}
	}
	return rows, nil
}

// parseSize turns Docker sizes such as "1.2GB" or "512kB" into bytes.
func parseSize(s string) float64 {
	s = strings.TrimSpace(strings.SplitN(s, " ", 2)[0])
	units := []struct {
		suffix string
		mult   float64
	}{{"TB", 1e12}, {"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"KB", 1e3}, {"B", 1}}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			v, err := strconv.ParseFloat(strings.TrimSuffix(s, u.suffix), 64)
			if err != nil {
				return 0
			}
			return v * u.mult
		}
	}
	return 0
}

func humanBytes(b float64) string {
	switch {
	case b >= 1e9:
		return fmt.Sprintf("%.1f GB", b/1e9)
	case b >= 1e6:
		return fmt.Sprintf("%.0f MB", b/1e6)
	case b >= 1e3:
		return fmt.Sprintf("%.0f kB", b/1e3)
	}
	return fmt.Sprintf("%.0f B", b)
}

func totalSize(rows []dockerDF) float64 {
	var t float64
	for _, r := range rows {
		t += parseSize(r.Size)
	}
	return t
}

func dockerRunning() bool { return exec.Command("docker", "info").Run() == nil }

func dockerDiskImage() string {
	home, _ := os.UserHomeDir()
	raw := filepath.Join(home, "Library/Containers/com.docker.docker/Data/vms/0/data/Docker.raw")
	out, err := exec.Command("du", "-h", raw).Output()
	if err != nil {
		return ""
	}
	return strings.Fields(string(out))[0]
}

func dockerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "docker",
		Short: i18n.T("cmd.docker"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !runner.Has("docker") {
				fmt.Println(i18n.T("docker.not_installed"))
				return nil
			}
			tty := term.IsTerminal(int(os.Stdin.Fd()))
			if !dockerRunning() {
				if !tty || !i18n.IsYes(ask(i18n.T("docker.not_running"), "n")) {
					return nil
				}
				fmt.Println(i18n.T("docker.starting"))
				start, _ := steps.Find("docker-start")
				if res := runner.Run(context.Background(), start.Command, 2*time.Minute, nil); res.ExitCode != 0 {
					return fmt.Errorf("docker: %s", strings.TrimSpace(res.Output))
				}
			}

			rows, err := dockerUsage()
			if err != nil {
				return fmt.Errorf("docker system df: %w", err)
			}
			before := totalSize(rows)
			reclaim := map[string]string{}
			fmt.Printf("\n%s\n\n", i18n.T("docker.usage"))
			fmt.Printf("  %-14s %7s %7s %10s  %s\n", "", "total", "active", "size", "reclaimable")
			for _, r := range rows {
				fmt.Printf("  %-14s %7s %7s %10s  %s\n", r.Type, r.TotalCount, r.Active, r.Size, r.Reclaimable)
				reclaim[r.Type] = strings.SplitN(r.Reclaimable, " ", 2)[0]
			}
			if img := dockerDiskImage(); img != "" {
				fmt.Println("\n  " + i18n.T("docker.disk_image", img))
			}
			fmt.Println()

			badge := func(t string) string {
				if v := reclaim[t]; v != "" && parseSize(v) > 0 {
					return i18n.T("docker.reclaimable", v)
				}
				return ""
			}
			pickable := []struct{ id, badge, risk string }{
				{"docker-containers", badge("Containers"), "normal"},
				{"docker-images", "", "normal"},
				{"docker-images-all", badge("Images"), "normal"},
				{"docker-build-cache", badge("Build Cache"), "normal"},
				{"docker-volumes", badge("Local Volumes"), "dangerous"},
			}
			var opts []ui.Option
			for _, p := range pickable {
				s, _ := steps.Find(p.id)
				opts = append(opts, ui.Option{ID: p.id, Label: i18n.TOr("step."+p.id, s.Name),
					Detail: s.Command, Badge: p.badge, Risk: p.risk})
			}
			ids, err := ui.Pick(i18n.T("docker.pick_title"), i18n.T("docker.pick_hint"), opts, true, tty, os.Stdin, os.Stdout)
			if errors.Is(err, ui.ErrCancelled) || (err == nil && len(ids) == 0) {
				fmt.Println(i18n.T("docker.nothing"))
				return nil
			}
			if err != nil {
				return err
			}
			// Docker cleanups are always confirmed, whatever the approval settings.
			if !i18n.IsYes(ask(i18n.T("docker.confirm", len(ids)), "n")) {
				fmt.Println(i18n.T("docker.nothing"))
				return nil
			}
			for _, id := range ids {
				s, _ := steps.Find(id)
				fmt.Printf("\n⏺ %s\n  %s\n", i18n.TOr("step."+id, s.Name), s.Command)
				res := runner.Run(context.Background(), s.Command, 30*time.Minute, func(l string) { fmt.Println("    " + l) })
				if res.ExitCode != 0 {
					fmt.Println("  " + i18n.T("steps.failed", id, res.ExitCode))
				} else {
					fmt.Println("  ⎿ ✓")
				}
			}
			if after, err := dockerUsage(); err == nil {
				fmt.Println("\n" + i18n.T("docker.freed", humanBytes(totalSize(after)), humanBytes(before)))
			}
			fmt.Println(i18n.T("docker.compact"))
			return nil
		},
	}
}
