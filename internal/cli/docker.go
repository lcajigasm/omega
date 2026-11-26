package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

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

func totalSize(rows []dockerDF) float64 {
	var t float64
	for _, r := range rows {
		t += parseSize(r.Size)
	}
	return t
}

var dockerRunning = func() bool { return exec.Command("docker", "info").Run() == nil }

var dockerInstalled = func() bool { return runner.Has("docker") }

func dockerDiskImage() string {
	home, _ := os.UserHomeDir()
	raw := filepath.Join(home, "Library/Containers/com.docker.docker/Data/vms/0/data/Docker.raw")
	out, err := exec.Command("du", "-k", raw).Output()
	if err != nil {
		return ""
	}
	f := strings.Fields(string(out))
	if len(f) == 0 {
		return ""
	}
	kb, err := strconv.ParseInt(f[0], 10, 64)
	if err != nil {
		return ""
	}
	return ui.Bytes(kb << 10)
}

// dockerReport is what docker --list --json prints.
type dockerReport struct {
	Rows      []dockerRow `json:"rows"`
	Total     int64       `json:"total"`       // bytes
	Reclaim   int64       `json:"reclaimable"` // bytes
	DiskImage string      `json:"disk_image,omitempty"`
}

type dockerRow struct {
	Type        string `json:"type"`
	Count       string `json:"count"`
	Active      string `json:"active"`
	Size        int64  `json:"size"`
	Reclaimable int64  `json:"reclaimable"`
	Percent     string `json:"percent,omitempty"`
}

func newDockerReport(rows []dockerDF) dockerReport {
	rep := dockerReport{Rows: []dockerRow{}}
	for _, r := range rows {
		row := dockerRow{Type: r.Type, Count: r.TotalCount, Active: r.Active,
			Size: int64(parseSize(r.Size)), Reclaimable: int64(parseSize(r.Reclaimable))}
		if _, pct, ok := strings.Cut(r.Reclaimable, " "); ok {
			row.Percent = strings.Trim(pct, "()")
		}
		rep.Total += row.Size
		rep.Reclaim += row.Reclaimable
		rep.Rows = append(rep.Rows, row)
	}
	return rep
}

func writeDockerUsage(w io.Writer, rep dockerReport) {
	r := ui.NewReport(w)
	r.Title("omega docker", i18n.T("docker.usage"))
	r.Blank()
	var rows [][]string
	for _, row := range rep.Rows {
		rec := ui.Bytes(row.Reclaimable)
		if row.Percent != "" {
			rec += r.Muted(" (" + row.Percent + ")")
		}
		rows = append(rows, []string{row.Type, row.Count, row.Active, ui.Bytes(row.Size), rec})
	}
	r.Table([]ui.Column{{Title: ""}, {Title: i18n.T("docker.col_total"), Right: true}, {Title: i18n.T("docker.col_active"), Right: true},
		{Title: i18n.T("disk.col_size"), Right: true}, {Title: i18n.T("docker.col_reclaimable"), Right: true}}, rows, nil)
	r.Blank()
	r.Line(i18n.T("docker.total", ui.Bytes(rep.Total), ui.Bytes(rep.Reclaim)))
	if rep.DiskImage != "" {
		r.Line(r.Muted(i18n.T("docker.disk_image", rep.DiskImage)))
	}
}

func dockerCmd() *cobra.Command {
	var list, asJSON, yes bool
	c := &cobra.Command{
		Use:     "docker",
		Short:   i18n.T("cmd.docker"),
		Long:    i18n.T("cmd.docker_long"),
		Example: "  omega docker\n  omega docker --list\n  omega docker --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			if asJSON {
				list = true
			}
			if !dockerInstalled() {
				return errors.New(i18n.T("docker.not_installed"))
			}
			tty := stdinIsTerminal() && w == io.Writer(os.Stdout)
			if !dockerRunning() {
				// Without a terminal nobody can say yes to starting it: say
				// what is wrong and fail, so a script sees it.
				if !tty || list {
					return errors.New(i18n.T("docker.stopped"))
				}
				if !askYesNo(i18n.T("docker.not_running")) {
					return errors.New(i18n.T("docker.stopped"))
				}
				fmt.Fprintln(w, i18n.T("docker.starting"))
				start, _ := steps.Find("docker-start")
				if res := runner.Run(context.Background(), start.Command, 2*time.Minute, nil); res.ExitCode != 0 {
					return fmt.Errorf("docker: %s", strings.TrimSpace(res.Output))
				}
			}

			rows, err := dockerUsage()
			if err != nil {
				return fmt.Errorf("docker system df: %w", err)
			}
			rep := newDockerReport(rows)
			rep.DiskImage = dockerDiskImage()
			if asJSON {
				return writeJSON(w, rep)
			}
			writeDockerUsage(w, rep)
			if list {
				return nil
			}
			if !tty {
				ui.NewReport(w).Hint(i18n.T("docker.need_tty"))
				return nil
			}
			before := totalSize(rows)
			reclaim := map[string]float64{}
			for _, r := range rows {
				reclaim[r.Type] = parseSize(r.Reclaimable)
			}
			badge := func(t string) string {
				if v := reclaim[t]; v > 0 {
					return i18n.T("docker.reclaimable", ui.Bytes(int64(v)))
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
			fmt.Fprintln(w)
			ids, err := ui.Pick(i18n.T("docker.pick_title"), i18n.T("docker.pick_hint"), opts, true, true, os.Stdin, w)
			if errors.Is(err, ui.ErrCancelled) || (err == nil && len(ids) == 0) {
				say(w, ui.MarkSkip, i18n.T("docker.nothing"))
				return nil
			}
			if err != nil {
				return err
			}
			// Docker cleanups are always confirmed, whatever the approval settings.
			if !yes && !askYesNo(i18n.N("docker.confirm", len(ids))) {
				say(w, ui.MarkSkip, i18n.T("docker.nothing"))
				return nil
			}
			r := ui.NewReport(w)
			failed := 0
			for _, id := range ids {
				s, _ := steps.Find(id)
				r.Blank()
				r.Check(ui.MarkInfo, i18n.TOr("step."+id, s.Name), 0, r.Muted(s.Command))
				res := runner.Run(context.Background(), s.Command, 30*time.Minute, func(l string) { r.Detail(l) })
				if res.ExitCode != 0 {
					failed++
					r.Check(ui.MarkErr, i18n.T("steps.failed", id, res.ExitCode), 0, "")
				} else {
					r.Check(ui.MarkOK, i18n.TOr("step."+id, s.Name), 0, "")
				}
			}
			r.Blank()
			if after, err := dockerUsage(); err == nil {
				r.Line(i18n.T("docker.freed", ui.Bytes(int64(totalSize(after))), ui.Bytes(int64(before))))
			}
			r.Hint(i18n.T("docker.compact"))
			if failed > 0 {
				return exitCode(1)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&list, "list", false, i18n.T("flag.docker_list"))
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.tools_json"))
	c.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("flag.docker_yes"))
	return c
}
