package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/ui"
)

// menuEntries is everything the home screen starts: the command line of
// each entry, grouped. The detail is the i18n key of its description.
var menuEntries = []struct{ group, args, detail string }{
	{"maintain", "", "slash.task_full"},
	{"maintain", "update", "slash.task_update"},
	{"maintain", "clean", "slash.task_clean"},
	{"maintain", "analyze", "slash.task_analyze"},
	{"maintain", "tune", "slash.task_tune"},
	{"maintain", "fix", "slash.task_fix"},
	{"maintain", "docker", "cmd.docker"},
	{"mac", "mac", "slash.task_mac"},
	{"mac", "mac status", "cmd.mac_status"},
	{"mac", "mac permissions", "cmd.mac_permissions"},
	{"mac", "mac apps", "cmd.mac_apps"},
	{"mac", "mac undo", "cmd.mac_undo"},
	{"dev", "dev", "slash.task_dev"},
	{"dev", "dev status", "cmd.dev_status"},
	{"dev", "dev install", "cmd.dev_install"},
	{"setup", "doctor", "cmd.doctor"},
	{"setup", "setup", "cmd.setup"},
	{"setup", "model", "cmd.model"},
	{"setup", "logs", "cmd.logs"},
	{"setup", "steps", "cmd.steps"},
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return s
}

// ago is a short "how long since" for the home screen.
func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return i18n.T("menu.ago_min", int(d.Minutes()))
	case d < 48*time.Hour:
		return i18n.T("menu.ago_hours", int(d.Hours()))
	}
	return i18n.T("menu.ago_days", int(d.Hours()/24))
}

func menuInfo() ui.MenuInfo {
	info := ui.MenuInfo{Version: version}
	if cfg, err := config.Load(); err == nil {
		info.Provider = cfg.Provider
		info.Model = cfg.ModelFor(cfg.Provider)
		if info.Model == "" {
			info.Model = "auto"
		}
		if p, ok := llm.Lookup(cfg.Provider); ok && !p.Local {
			if key, _, _ := getKey(cfg.Provider); key == "" {
				info.Notes = append(info.Notes, i18n.T("menu.no_key"))
			}
		}
	}
	if logs, err := agent.ListRunLogs(); err == nil && len(logs) > 0 {
		info.LastRun = ago(logs[0].ModTime)
	}
	return info
}

// menuCmd is the home screen: what omega can do, started with Enter. Each
// entry runs the same command the user could type, and comes back here.
func menuCmd() *cobra.Command {
	return &cobra.Command{
		Use: "menu", Aliases: []string{"home"}, Short: i18n.T("cmd.menu"), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
				return fmt.Errorf("%s", i18n.T("menu.need_tty"))
			}
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			var items []ui.MenuItem
			for _, e := range menuEntries {
				if e.group == "mac" && goos != "darwin" {
					continue
				}
				items = append(items, ui.MenuItem{ID: e.args, Group: i18n.T("menu.g." + e.group),
					Label: strings.TrimSpace("omega " + e.args), Detail: firstLine(i18n.T(e.detail))})
			}
			in := bufio.NewReader(os.Stdin)
			for {
				picked, err := ui.RunMenuOrQuit(menuInfo(), items)
				if err != nil || picked == ui.MenuQuit {
					return err
				}
				c := exec.Command(exe, strings.Fields(picked)...)
				c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
				if err := c.Run(); err != nil {
					if _, exited := err.(*exec.ExitError); !exited {
						return err
					}
				}
				fmt.Print("\n" + i18n.T("menu.back"))
				if _, err := in.ReadString('\n'); err != nil {
					return nil
				}
			}
		},
	}
}
