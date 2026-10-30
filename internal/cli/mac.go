package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/macos"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/sudo"
	"github.com/lcajigasm/omega/internal/ui"
)

func macCmd() *cobra.Command {
	var f runFlags
	c := &cobra.Command{
		Use: "mac", Short: i18n.T("cmd.mac"), Long: i18n.T("cmd.mac_long"), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runTask(agent.TaskMac, f) },
	}
	bindRunFlags(c.Flags(), &f)
	c.Flags().MarkHidden("prompt")

	section := func(use, short string, run func(inv *macos.Inventory, tty bool) error, sections ...string) *cobra.Command {
		return &cobra.Command{Use: use, Short: i18n.T(short), Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				inv, err := loadInventory(sections...)
				if err != nil {
					return err
				}
				return run(inv, term.IsTerminal(int(os.Stdin.Fd())))
			}}
	}
	c.AddCommand(
		section("status", "cmd.mac_status", func(inv *macos.Inventory, _ bool) error { printStatus(inv); return nil }),
		section("login", "cmd.mac_login", macLogin, "login"),
		section("background", "cmd.mac_background", macBackground, "background"),
		section("menubar", "cmd.mac_menubar", macMenuBar, "menubar"),
		section("dock", "cmd.mac_dock", macDock, "dock"),
		section("shortcuts", "cmd.mac_shortcuts", macShortcuts, "shortcuts"),
		macPolicyCmd(), macUndoCmd(), macOpCmd(),
	)
	return c
}

func loadInventory(sections ...string) (*macos.Inventory, error) {
	p, err := macos.LoadPolicy()
	if err != nil {
		return nil, err
	}
	needBTM := len(sections) == 0
	for _, s := range sections {
		needBTM = needBTM || s == "login" || s == "background" || s == "menubar"
	}
	if needBTM {
		fmt.Println(i18n.T("mac.reading"))
	}
	inv, err := macos.Load(p, sections...)
	if err == nil && inv.NoBTM && needBTM {
		fmt.Println(i18n.T("mac.no_btm"))
	}
	return inv, err
}

func verdictBadge(v string) string { return i18n.T("mac.verdict." + v) }

func verdictRisk(v string) string {
	switch v {
	case macos.Disable:
		return "normal"
	case macos.Review:
		return "dangerous"
	}
	return "read-only"
}

func itemDetail(it macos.Item) string {
	parts := []string{i18n.T("mac.kind." + it.Kind), i18n.T("mac.cat." + it.Category)}
	if it.Developer != "" {
		parts = append(parts, it.Developer)
	}
	if it.Running {
		parts = append(parts, i18n.T("mac.running"))
	}
	if it.Missing {
		parts = append(parts, i18n.T("mac.missing"))
	}
	if it.System {
		parts = append(parts, i18n.T("mac.sudo"))
	}
	if !it.Enabled {
		parts = append(parts, i18n.T("mac.off"))
	}
	return strings.Join(parts, " · ")
}

// printStatus is the read-only overview of every section.
func printStatus(inv *macos.Inventory) {
	fmt.Printf("\n%s\n", i18n.T("mac.h.login"))
	for _, it := range inv.LoginItems() {
		fmt.Printf("  %-34s %-14s %s\n", cut(it.Name, 34), verdictBadge(it.Verdict), i18n.T("mac.cat."+it.Category))
	}
	fmt.Printf("\n%s\n", i18n.T("mac.h.background"))
	for _, it := range inv.BackgroundItems() {
		fmt.Printf("  %-34s %-14s %s\n", cut(it.Name, 34), verdictBadge(it.Verdict), itemDetail(it))
	}
	fmt.Printf("\n%s\n", i18n.T("mac.h.menubar"))
	for _, m := range inv.MenuBar {
		fmt.Printf("  %-34s %-14s %s\n", cut(m.Name, 34), verdictBadge(m.Verdict), i18n.T("mac.cat."+m.Category))
	}
	if inv.Manager != "" {
		fmt.Println("  " + i18n.T("mac.manager", inv.Manager))
	}
	if d := inv.Dock; d != nil {
		fmt.Printf("\n%s\n", i18n.T("mac.h.dock"))
		remove, why, _ := inv.DockSuggestions()
		suggested := map[string]bool{}
		for _, a := range remove {
			suggested[a.Label] = true
		}
		for _, a := range d.Apps {
			note := ""
			if suggested[a.Label] {
				note = dockWhy(inv, a, why[a.Label])
			}
			fmt.Printf("  %2d. %-30s %s\n", a.Index+1, a.Label, note)
		}
		fmt.Println("  " + i18n.T("mac.dock_settings", d.Autohide, d.ShowRecents, d.TileSize))
	}
	fmt.Printf("\n%s\n", i18n.T("mac.h.shortcuts"))
	if len(inv.Conflicts) == 0 {
		fmt.Println("  " + i18n.T("mac.no_conflicts"))
	}
	for _, c := range inv.Conflicts {
		fmt.Println("  " + i18n.T("mac.conflict", c.Combo.String()))
		for i, h := range c.Hotkeys {
			mark := " "
			if i == c.Winner {
				mark = "★"
			}
			fmt.Printf("    %s %-14s %s\n", mark, h.Owner, h.Action)
		}
	}
	fmt.Println("\n" + i18n.T("mac.status_next"))
}

// cut shortens s to n characters so the columns stay aligned.
func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func dockWhy(inv *macos.Inventory, a macos.DockApp, why string) string {
	switch why {
	case "missing":
		return i18n.T("mac.not_installed")
	case "rules":
		return i18n.T("mac.rules")
	case "unused":
		return i18n.T("mac.unused", int(inv.Now.Sub(a.LastUsed).Hours()/24))
	}
	return ""
}

// pickChanges shows a card of changes and returns the chosen ones.
func pickChanges(title string, opts []ui.Option, changes map[string]macos.Change, tty bool) []macos.Change {
	if len(opts) == 0 {
		fmt.Println(i18n.T("mac.none_found"))
		return nil
	}
	ids, err := ui.Pick(title, i18n.T("mac.pick_hint"), opts, true, tty, os.Stdin, os.Stdout)
	if errors.Is(err, ui.ErrCancelled) || err != nil || len(ids) == 0 {
		fmt.Println(i18n.T("mac.nothing"))
		return nil
	}
	var out []macos.Change
	for _, id := range ids {
		if c, ok := changes[id]; ok {
			out = append(out, c)
		}
	}
	return out
}

// applyChanges runs the changes, records them for undo and says what is
// left to do by hand.
func applyChanges(changes []macos.Change) error {
	var bridge *sudo.Bridge
	defer func() {
		if bridge != nil {
			bridge.Close()
			runner.Configure(nil, "")
		}
	}()
	failed := 0
	for _, c := range changes {
		fmt.Printf("\n⏺ %s\n  %s\n", c.Title, c.Command)
		if c.Sudo() && bridge == nil {
			b, err := sudo.Start(askTerminalPassword, "")
			if err != nil {
				return err
			}
			bridge = b
			runner.Configure(b.Env(), b.PathPrefix())
		}
		res := runner.Run(context.Background(), c.Command, 2*time.Minute, func(l string) { fmt.Println("    " + l) })
		if res.ExitCode != 0 {
			failed++
			fmt.Println("  " + i18n.T("mac.failed", res.ExitCode))
		} else {
			fmt.Println("  ⎿ ✓")
			if err := macos.Record(c); err != nil {
				fmt.Println("  " + err.Error())
			}
		}
		if c.Manual != "" {
			fmt.Println("  ↳ " + c.Manual)
		}
	}
	if len(changes) > 0 {
		fmt.Println("\n" + i18n.T("mac.undo_hint"))
	}
	if failed > 0 {
		return fmt.Errorf("%s", i18n.T("steps.failed_summary", failed))
	}
	return nil
}

func askTerminalPassword(_ string, retry bool) (string, bool) {
	if retry {
		fmt.Println(i18n.T("sudo.wrong"))
	}
	pw, err := askSecret(i18n.T("sudo.prompt"))
	return pw, err == nil && pw != ""
}

func itemOptions(items []macos.Item) ([]ui.Option, map[string]macos.Change) {
	var opts []ui.Option
	changes := map[string]macos.Change{}
	for _, it := range items {
		if !it.Enabled || it.Category == macos.CatSystem {
			continue
		}
		c := it.DisableChange()
		changes[c.ID] = c
		opts = append(opts, ui.Option{ID: c.ID, Label: c.Title, Detail: itemDetail(it), Badge: verdictBadge(it.Verdict),
			Risk: verdictRisk(it.Verdict), Selected: it.Verdict == macos.Disable})
	}
	return opts, changes
}

func macLogin(inv *macos.Inventory, tty bool) error {
	opts, changes := itemOptions(inv.LoginItems())
	return applyChanges(pickChanges(i18n.T("mac.pick.login"), opts, changes, tty))
}

func macBackground(inv *macos.Inventory, tty bool) error {
	opts, changes := itemOptions(inv.BackgroundItems())
	return applyChanges(pickChanges(i18n.T("mac.pick.background"), opts, changes, tty))
}

func macMenuBar(inv *macos.Inventory, tty bool) error {
	var opts []ui.Option
	changes := map[string]macos.Change{}
	logins := map[string]macos.Item{}
	for _, it := range inv.Items {
		if it.Enabled && it.Bundle != "" && (it.Kind == macos.KindLogin || it.Kind == macos.KindHelper) {
			logins[it.Bundle] = it
		}
	}
	for _, m := range inv.MenuBar {
		q := m.QuitChange()
		changes[q.ID] = q
		opts = append(opts, ui.Option{ID: q.ID, Label: q.Title, Detail: i18n.T("mac.cat." + m.Category), Badge: verdictBadge(m.Verdict),
			Risk: verdictRisk(m.Verdict), Selected: m.Verdict == macos.Disable})
		if it, ok := logins[m.Bundle]; ok {
			d := it.DisableChange()
			changes[d.ID] = d
			opts = append(opts, ui.Option{ID: d.ID, Label: d.Title, Detail: itemDetail(it), Risk: "normal", Selected: m.Verdict == macos.Disable})
		}
	}
	if len(inv.MenuBar) > 0 {
		h := inv.MenuBar[0].HideChange(inv.Manager)
		h.ID, h.Title = "hide:", i18n.T("mac.act.hide_icons")
		changes[h.ID] = h
		opts = append(opts, ui.Option{ID: h.ID, Label: h.Title, Detail: h.Manual, Risk: "read-only"})
	}
	if inv.Manager != "" {
		fmt.Println(i18n.T("mac.manager", inv.Manager))
	}
	return applyChanges(pickChanges(i18n.T("mac.pick.menubar"), opts, changes, tty))
}

func macDock(inv *macos.Inventory, tty bool) error {
	d := inv.Dock
	remove, why, add := inv.DockSuggestions()
	suggested := map[string]string{}
	for _, a := range remove {
		suggested[a.Label] = why[a.Label]
	}
	var opts []ui.Option
	changes := map[string]macos.Change{}
	for _, a := range d.Apps {
		c := a.RemoveChange()
		changes[c.ID] = c
		w, ok := suggested[a.Label]
		o := ui.Option{ID: c.ID, Label: c.Title, Detail: a.Path, Risk: "read-only", Selected: ok && w != "unused"}
		if ok {
			o.Badge, o.Risk = dockWhy(inv, a, w), "normal"
		}
		opts = append(opts, o)
	}
	for _, a := range add {
		c := macos.AddChange(a)
		changes[c.ID] = c
		opts = append(opts, ui.Option{ID: c.ID, Label: c.Title, Detail: a.Path, Badge: i18n.T("mac.rules"), Risk: "normal", Selected: true})
	}
	settings := map[string]*bool{"autohide": inv.Policy.Dock.Autohide, "show-recents": inv.Policy.Dock.ShowRecents}
	current := map[string]bool{"autohide": d.Autohide, "show-recents": d.ShowRecents}
	for _, key := range []string{"autohide", "show-recents"} {
		if want := settings[key]; want != nil && *want != current[key] {
			if c, err := d.SettingChange(key, strconv.FormatBool(*want)); err == nil {
				changes[c.ID] = c
				opts = append(opts, ui.Option{ID: c.ID, Label: c.Title, Badge: i18n.T("mac.rules"), Risk: "normal", Selected: true})
			}
		}
	}
	return applyChanges(pickChanges(i18n.T("mac.pick.dock"), opts, changes, tty))
}

// macShortcuts asks who keeps each contested shortcut, moves the others to a
// free combo (or says where to change them by hand) and remembers the choice.
func macShortcuts(inv *macos.Inventory, tty bool) error {
	if len(inv.Conflicts) == 0 {
		fmt.Println(i18n.T("mac.no_conflicts"))
		return nil
	}
	used := macos.Used(inv.Hotkeys)
	var todo []macos.Change
	saved := false
	for _, c := range inv.Conflicts {
		var opts []ui.Option
		order := append([]int{c.Winner}, others(len(c.Hotkeys), c.Winner)...)
		for _, i := range order {
			if i < 0 {
				continue
			}
			h := c.Hotkeys[i]
			o := ui.Option{ID: strconv.Itoa(i), Label: h.Owner + " · " + h.Action, Risk: "read-only"}
			if i == c.Winner {
				o.Badge = i18n.T("mac.suggested")
			}
			if h.Assumed {
				o.Detail = i18n.T("mac.assumed")
			}
			opts = append(opts, o)
		}
		opts = append(opts, ui.Option{ID: "none", Label: i18n.T("mac.nobody"), Risk: "normal"})
		ids, err := ui.Pick(i18n.T("mac.pick.shortcut", c.Combo.String()), i18n.T("mac.pick_owner_hint"), opts, false, tty, os.Stdin, os.Stdout)
		if err != nil || len(ids) == 0 {
			continue
		}
		owner := "none"
		keep := -1
		if ids[0] != "none" {
			keep, _ = strconv.Atoi(ids[0])
			owner = c.Hotkeys[keep].Owner
		}
		for i, h := range c.Hotkeys {
			if i == keep {
				continue
			}
			if alts := macos.Alternatives(h, used, 1); len(alts) > 0 && owner != "none" {
				used[alts[0].Config()] = true
				todo = append(todo, h.SetChange(alts[0]))
			} else {
				todo = append(todo, h.DisableChange())
			}
		}
		if err := inv.Policy.AddRule("owner", c.Combo.Config()+"="+owner); err == nil {
			saved = true
		}
	}
	if saved {
		if err := inv.Policy.Save(); err == nil {
			fmt.Println(i18n.T("mac.saved_rule", macos.PolicyPath()))
		}
	}
	if len(todo) == 0 {
		fmt.Println(i18n.T("mac.nothing"))
		return nil
	}
	var opts []ui.Option
	changes := map[string]macos.Change{}
	for _, c := range todo {
		changes[c.ID] = c
		o := ui.Option{ID: c.ID, Label: c.Title, Detail: c.Manual, Risk: "normal", Selected: true}
		if c.Manual != "" {
			o.Badge = i18n.T("mac.by_hand")
		}
		opts = append(opts, o)
	}
	return applyChanges(pickChanges(i18n.T("mac.pick.changes"), opts, changes, tty))
}

func others(n, skip int) []int {
	var out []int
	for i := 0; i < n; i++ {
		if i != skip {
			out = append(out, i)
		}
	}
	return out
}

func macPolicyCmd() *cobra.Command {
	c := &cobra.Command{
		Use: "policy", Short: i18n.T("cmd.mac_policy"), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(i18n.T("mac.policy_defaults"))
			b, err := os.ReadFile(macos.PolicyPath())
			if err != nil {
				fmt.Println("\n" + i18n.T("mac.policy_none"))
				return nil
			}
			fmt.Printf("\n%s\n\n%s", i18n.T("mac.policy_path", macos.PolicyPath()), b)
			return nil
		},
	}
	c.AddCommand(&cobra.Command{
		Use: "edit", Short: i18n.T("cmd.mac_policy_edit"), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := os.Stat(macos.PolicyPath()); err != nil {
				p, _ := macos.LoadPolicy()
				if err := p.Save(); err != nil {
					return err
				}
			}
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "nano"
			}
			e := exec.Command(editor, macos.PolicyPath())
			e.Stdin, e.Stdout, e.Stderr = os.Stdin, os.Stdout, os.Stderr
			return e.Run()
		},
	})
	return c
}

func macUndoCmd() *cobra.Command {
	return &cobra.Command{
		Use: "undo", Short: i18n.T("cmd.mac_undo"), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var opts []ui.Option
			entries := map[string]macos.Entry{}
			for _, e := range macos.Journal() {
				if e.Undone || e.Undo == "" || len(opts) >= 40 {
					continue
				}
				id := e.Time.Format(time.RFC3339Nano)
				entries[id] = e
				opts = append(opts, ui.Option{ID: id, Label: e.Title, Detail: e.Undo, Badge: e.Time.Format("2006-01-02 15:04"), Risk: "normal"})
			}
			if len(opts) == 0 {
				fmt.Println(i18n.T("mac.undo_empty"))
				return nil
			}
			ids, err := ui.Pick(i18n.T("mac.pick.undo"), i18n.T("mac.pick_hint"), opts, true, term.IsTerminal(int(os.Stdin.Fd())), os.Stdin, os.Stdout)
			if err != nil || len(ids) == 0 {
				fmt.Println(i18n.T("mac.nothing"))
				return nil
			}
			done := map[time.Time]bool{}
			failed := 0
			for _, id := range ids {
				e := entries[id]
				fmt.Printf("\n⏺ %s\n  %s\n", i18n.T("mac.undoing", e.Title), e.Undo)
				res := runner.Run(context.Background(), e.Undo, 2*time.Minute, func(l string) { fmt.Println("    " + l) })
				if res.ExitCode != 0 {
					failed++
					fmt.Println("  " + i18n.T("mac.failed", res.ExitCode))
					continue
				}
				fmt.Println("  ⎿ ✓")
				done[e.Time] = true
				if e.Section == "login" || e.Section == "background" {
					macos.ForgetBTM()
				}
			}
			if err := macos.MarkUndone(done); err != nil {
				return err
			}
			if failed > 0 {
				return fmt.Errorf("%s", i18n.T("steps.failed_summary", failed))
			}
			return nil
		},
	}
}

// macOpCmd runs the Dock edits that are not a single shell command; the
// changes omega shows and journals call it.
func macOpCmd() *cobra.Command {
	return &cobra.Command{
		Use: "op <dock-remove|dock-add> <app> [index]", Hidden: true, Args: cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "dock-remove":
				return macos.DockRemove(args[1])
			case "dock-add":
				index := -1
				if len(args) == 3 {
					index, _ = strconv.Atoi(args[2])
				}
				return macos.DockAdd(args[1], index)
			}
			return fmt.Errorf("unknown op %q", args[0])
		},
	}
}
