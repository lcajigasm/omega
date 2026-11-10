package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/devtools"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/ui"
)

func devCmd() *cobra.Command {
	var f runFlags
	c := &cobra.Command{
		Use: "dev", Short: i18n.T("cmd.dev"), Long: i18n.T("cmd.dev_long"), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runTask(agent.TaskDev, f) },
	}
	bindRunFlags(c.Flags(), &f)
	c.Flags().MarkHidden("prompt")
	c.AddCommand(
		&cobra.Command{Use: "status", Short: i18n.T("cmd.dev_status"), Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Println(i18n.T("dev.reading"))
				printDev(devtools.Load(true))
				return nil
			}},
		&cobra.Command{Use: "install [tool]...", Short: i18n.T("cmd.dev_install"),
			ValidArgsFunction: func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
				var ids []string
				for _, t := range devtools.Catalog {
					if t.Install != "" {
						ids = append(ids, t.ID)
					}
				}
				return ids, cobra.ShellCompDirectiveNoFileComp
			},
			RunE: func(cmd *cobra.Command, args []string) error {
				return devInstall(args, term.IsTerminal(int(os.Stdin.Fd())))
			}},
		&cobra.Command{Use: "activate", Short: i18n.T("cmd.dev_activate"), Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				installed, _ := devtools.Detect()
				return devActivate(installed, term.IsTerminal(int(os.Stdin.Fd())))
			}},
	)
	return c
}

// printDev is the read-only overview of the development setup.
func printDev(inv *devtools.Inventory) {
	for _, cat := range devtools.Categories {
		first := true
		for _, t := range inv.Installed {
			if t.Category != cat {
				continue
			}
			if first {
				fmt.Printf("\n%s\n", i18n.T("dev.cat."+cat))
				first = false
			}
			where := t.Manager
			if where == "" {
				where = homeShort(t.Path)
			}
			note := ""
			if t.Inactive {
				note = "  ⚠ " + i18n.T("dev.inactive")
			}
			fmt.Printf("  %-18s %-16s %s%s\n", cut(t.Name, 18), cut(orDash(t.Version), 16), where, note)
		}
	}
	fmt.Printf("\n%s\n", i18n.T("dev.h.findings"))
	if inv.Findings() == 0 {
		fmt.Println("  ✓ " + i18n.T("dev.no_findings"))
	}
	for _, c := range inv.Conflicts {
		fmt.Println("  ⚠ " + i18n.T("dev.conflict", c.Lang, strings.Join(c.Managers, ", ")))
	}
	for _, s := range inv.Shadows {
		fmt.Println("  ⚠ " + i18n.T("dev.shadow", s.Bin, strings.Join(homeShortAll(s.Paths), "  >  ")))
	}
	if len(inv.PathDup) > 0 {
		fmt.Println("  ⚠ " + i18n.T("dev.path_dup", strings.Join(homeShortAll(inv.PathDup), ", ")))
	}
	if len(inv.PathGone) > 0 {
		fmt.Println("  ⚠ " + i18n.T("dev.path_gone", strings.Join(homeShortAll(inv.PathGone), ", ")))
	}
	for _, t := range inv.Inactive() {
		fmt.Println("  ⚠ " + i18n.T("dev.not_active", t.Name, t.Init))
	}
	if inv.Startup > 0 {
		mark := "✓"
		if inv.Startup > devtools.SlowStartup {
			mark = "⚠"
		}
		fmt.Printf("  %s %s\n", mark, i18n.T("dev.startup", inv.Startup.Milliseconds()))
	}
	if len(inv.Stores) > 0 {
		fmt.Printf("\n%s\n", i18n.T("dev.h.versions"))
		for _, s := range inv.Stores {
			fmt.Printf("  %-28s %9s  %s\n", cut(s.Name, 28), devtools.HumanSize(s.Size), cut(strings.Join(s.Versions, ", "), 70))
		}
	}
	fmt.Println("\n" + i18n.T("dev.status_next", len(inv.Missing)))
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func homeShort(p string) string {
	if h, err := os.UserHomeDir(); err == nil && h != "" && strings.HasPrefix(p, h) {
		return "~" + strings.TrimPrefix(p, h)
	}
	return p
}

func homeShortAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = homeShort(s)
	}
	return out
}

// devInstall installs the named tools, or shows a card with everything the
// catalogue has that is not installed. Tools that need a line in ~/.zshrc
// are offered for activation afterwards.
func devInstall(names []string, tty bool) error {
	var tools []devtools.Tool
	if len(names) == 0 {
		_, missing := devtools.Detect()
		if len(missing) == 0 {
			fmt.Println(i18n.T("dev.all_installed"))
			return nil
		}
		var opts []ui.Option
		byID := map[string]devtools.Tool{}
		for _, cat := range devtools.Categories {
			for _, t := range missing {
				if t.Category == cat {
					byID[t.ID] = t
					opts = append(opts, ui.Option{ID: t.ID, Label: t.Name + " — " + t.About, Detail: t.Install, Badge: i18n.T("dev.cat." + cat), Risk: "normal"})
				}
			}
		}
		ids, err := ui.Pick(i18n.T("dev.pick.install"), i18n.T("mac.pick_hint"), opts, true, tty, os.Stdin, os.Stdout)
		if errors.Is(err, ui.ErrCancelled) || err != nil || len(ids) == 0 {
			fmt.Println(i18n.T("mac.nothing"))
			return nil
		}
		for _, id := range ids {
			tools = append(tools, byID[id])
		}
	}
	for _, n := range names {
		t, ok := devtools.Find(n)
		if !ok {
			return fmt.Errorf("%s", i18n.T("dev.unknown", n))
		}
		if t.Install == "" {
			return fmt.Errorf("%s", i18n.T("dev.no_install", t.Name))
		}
		if t.IsInstalled() {
			fmt.Println(i18n.T("dev.already", t.Name))
			continue
		}
		tools = append(tools, t)
	}
	if !runner.Has("brew") {
		for _, t := range tools {
			if strings.HasPrefix(t.Install, "brew ") {
				return fmt.Errorf("%s", i18n.T("dev.need_brew"))
			}
		}
	}
	failed := 0
	var done []devtools.Tool
	for _, t := range tools {
		fmt.Printf("\n⏺ %s\n  %s\n", t.Name, t.Install)
		res := runner.Run(context.Background(), t.Install, 30*time.Minute, func(l string) { fmt.Println("    " + l) })
		if res.ExitCode != 0 {
			failed++
			fmt.Println("  " + i18n.T("mac.failed", res.ExitCode))
			continue
		}
		fmt.Println("  ⎿ ✓")
		done = append(done, t)
	}
	if err := devActivate(done, tty); err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("%s", i18n.T("steps.failed_summary", failed))
	}
	return nil
}

// devActivate offers to add the shell lines of installed tools that are not
// in ~/.zshrc yet. The file is copied before it is touched.
func devActivate(tools []devtools.Tool, tty bool) error {
	inv := devtools.Load(false)
	want := map[string]bool{}
	for _, t := range tools {
		want[t.ID] = true
	}
	var opts []ui.Option
	byID := map[string]devtools.Tool{}
	for _, t := range inv.Inactive() {
		if want[t.ID] {
			byID[t.ID] = t.Tool
			opts = append(opts, ui.Option{ID: t.ID, Label: t.Name, Detail: t.Init, Risk: "normal", Selected: true})
		}
	}
	if len(opts) == 0 {
		return nil
	}
	if shell := os.Getenv("SHELL"); !strings.HasSuffix(shell, "zsh") {
		fmt.Println("\n" + i18n.T("dev.activate_other"))
		for _, o := range opts {
			fmt.Printf("  %-12s %s\n", o.Label, o.Detail)
		}
		return nil
	}
	ids, err := ui.Pick(i18n.T("dev.pick.activate"), i18n.T("mac.pick_hint"), opts, true, tty, os.Stdin, os.Stdout)
	if err != nil || len(ids) == 0 {
		return nil
	}
	var picked []devtools.Tool
	for _, id := range ids {
		picked = append(picked, byID[id])
	}
	added, backup, err := devtools.AddInit(picked, time.Now())
	if err != nil {
		return err
	}
	for _, t := range added {
		fmt.Printf("  + %s\n", t.Init)
	}
	if backup != "" {
		fmt.Println(i18n.T("dev.activated", homeShort(backup)))
	} else {
		fmt.Println(i18n.T("dev.activated_new"))
	}
	return nil
}
