package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
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
		return &cobra.Command{Use: use, Short: i18n.T(short), Long: i18n.TOr(short+"_more", ""), Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				// The title goes first: reading the login items can take
				// a while, and what is being read should be clear by then.
				ui.NewReport(os.Stdout).Title(macPath(cmd), i18n.T(short))
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
		section("permissions", "cmd.mac_permissions", macPermissions, "permissions"),
		section("apps", "cmd.mac_apps", macApps, "apps"),
		macPolicyCmd(), macUndoCmd(), macOpCmd(),
	)
	macOnly(c)
	return c
}

// goos is the operating system omega runs on. Tests replace it.
var goos = runtime.GOOS

// macOnly makes a command and its subcommands say that they need macOS
// when run anywhere else, before they touch anything.
func macOnly(c *cobra.Command) {
	if run := c.RunE; run != nil {
		c.RunE = func(cmd *cobra.Command, args []string) error {
			if goos != "darwin" {
				return fmt.Errorf("%s", i18n.T("mac.only", macPath(cmd), osLabel()))
			}
			return run(cmd, args)
		}
	}
	for _, sub := range c.Commands() {
		macOnly(sub)
	}
}

// macPath is the command as the user typed it: "omega mac undo".
func macPath(cmd *cobra.Command) string {
	path := cmd.CommandPath()
	if !strings.HasPrefix(path, "omega") {
		path = "omega " + path
	}
	return path
}

// osLabel names the operating system for messages.
func osLabel() string {
	switch goos {
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	}
	return goos
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
	r := ui.NewReport(os.Stdout)
	if needBTM {
		fmt.Println(r.Faint(i18n.T("mac.reading")))
	}
	inv, err := macos.Load(p, sections...)
	if err == nil && inv.NoBTM && needBTM {
		macNotice(r, ui.MarkWarn, i18n.T("mac.no_btm"))
	}
	if err == nil && inv.NoTCC {
		macNotice(r, ui.MarkWarn, i18n.T("mac.no_tcc"))
		offerFullDiskAccess()
	}
	return inv, err
}

// macNotice writes a sentence of omega mac with its mark in front, wrapped
// under itself to the terminal.
func macNotice(r *ui.Report, m ui.Mark, text string) {
	lines := strings.Split(lipgloss.NewStyle().Width(r.Width()-2).Render(text), "\n")
	for i, l := range lines {
		lead := "  "
		if i == 0 {
			lead = r.Glyph(m) + " "
		}
		fmt.Println(lead + strings.TrimRight(l, " "))
	}
}

// verdictMark is the mark of a verdict in a list: kept things are fine,
// the rest wants a look.
func verdictMark(v string) ui.Mark {
	if v == macos.Keep {
		return ui.MarkOK
	}
	return ui.MarkWarn
}

// verdictWord is the verdict, coloured when it suggests a change.
func verdictWord(r *ui.Report, v string) string {
	if v == macos.Keep {
		return r.Muted(verdictBadge(v))
	}
	return r.Warn(verdictBadge(v))
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
	r := ui.NewReport(os.Stdout)
	// The login items, the background ones and the menu bar line up with
	// each other, so the verdicts read as one column.
	nameW, verdictW := 0, 0
	for _, it := range inv.Items {
		nameW = max(nameW, lipgloss.Width(cut(it.Name, 34)))
		verdictW = max(verdictW, lipgloss.Width(verdictBadge(it.Verdict)))
	}
	for _, m := range inv.MenuBar {
		nameW = max(nameW, lipgloss.Width(cut(m.Name, 34)))
		verdictW = max(verdictW, lipgloss.Width(verdictBadge(m.Verdict)))
	}
	cols := []ui.Column{{Min: nameW}, {Min: verdictW}, {Wrap: true}}
	items := func(list []macos.Item, detail func(macos.Item) string) {
		var rows [][]string
		var marks []ui.Mark
		for _, it := range list {
			rows = append(rows, []string{cut(it.Name, 34), verdictWord(r, it.Verdict), r.Muted(detail(it))})
			marks = append(marks, verdictMark(it.Verdict))
		}
		if len(rows) == 0 {
			r.Line(r.Glyph(ui.MarkOff) + " " + r.Muted(i18n.T("mac.none_found")))
			return
		}
		r.Table(cols, rows, marks)
	}
	category := func(it macos.Item) string { return i18n.T("mac.cat." + it.Category) }

	login := inv.LoginItems()
	r.Section(i18n.T("mac.h.login"), len(login))
	items(login, category)
	background := inv.BackgroundItems()
	r.Section(i18n.T("mac.h.background"), len(background))
	items(background, itemDetail)

	r.Section(i18n.T("mac.h.menubar"), len(inv.MenuBar))
	var rows [][]string
	var marks []ui.Mark
	for _, m := range inv.MenuBar {
		rows = append(rows, []string{cut(m.Name, 34), verdictWord(r, m.Verdict), r.Muted(i18n.T("mac.cat." + m.Category))})
		marks = append(marks, verdictMark(m.Verdict))
	}
	r.Table(cols, rows, marks)
	if inv.Manager != "" {
		r.Line(r.Glyph(ui.MarkInfo) + " " + r.Muted(i18n.T("mac.manager", inv.Manager)))
	}

	if d := inv.Dock; d != nil {
		r.Section(i18n.T("mac.h.dock"), len(d.Apps))
		remove, why, _ := inv.DockSuggestions()
		suggested := map[string]bool{}
		for _, a := range remove {
			suggested[a.Label] = true
		}
		rows, marks = nil, nil
		for _, a := range d.Apps {
			note, mark := "", ui.MarkNone
			if suggested[a.Label] {
				note, mark = r.Warn(dockWhy(inv, a, why[a.Label])), ui.MarkWarn
			}
			rows = append(rows, []string{fmt.Sprint(a.Index + 1), cut(a.Label, 34), note})
			marks = append(marks, mark)
		}
		r.Table([]ui.Column{{Right: true}, {}, {}}, rows, marks)
		r.Line(r.Muted(i18n.T("mac.dock_settings", d.Autohide, d.ShowRecents, d.TileSize)))
	}

	r.Section(i18n.T("mac.h.shortcuts"), len(inv.Conflicts))
	if len(inv.Conflicts) == 0 {
		r.Line(r.Glyph(ui.MarkOK) + " " + i18n.T("mac.no_conflicts"))
	}
	for _, c := range inv.Conflicts {
		r.Line(r.Glyph(ui.MarkWarn) + " " + r.Bold(i18n.T("mac.conflict", c.Combo.String())))
		w := 0
		for _, h := range c.Hotkeys {
			w = max(w, lipgloss.Width(h.Owner))
		}
		for i, h := range c.Hotkeys {
			mark := ui.MarkNone
			if i == c.Winner {
				mark = ui.MarkCurrent
			}
			fmt.Printf("    %s %-*s  %s\n", r.Glyph(mark), w, h.Owner, r.Muted(h.Action))
		}
	}

	stale := inv.StalePermissions()
	r.Section(i18n.T("mac.h.permissions"), len(stale))
	r.Line(r.Muted(i18n.T("mac.perm_summary", len(inv.Permissions), len(stale))))
	rows, marks = nil, nil
	for _, x := range stale {
		rows = append(rows, []string{cut(x.Name, 34), cut(macos.ServiceName(x.Service), 30), r.Muted(permWhy(x))})
		marks = append(marks, ui.MarkWarn)
	}
	r.Table([]ui.Column{{}, {}, {Wrap: true}}, rows, marks)

	apps := inv.ReviewApps()
	r.Section(i18n.T("mac.h.apps"), len(apps))
	if len(apps) == 0 {
		r.Line(r.Glyph(ui.MarkOK) + " " + i18n.T("mac.none_found"))
	}
	rows, marks = nil, nil
	for _, a := range apps {
		rows = append(rows, []string{cut(a.Name, 34), macos.HumanSize(a.Size), r.Muted(appWhy(inv, a))})
		marks = append(marks, ui.MarkWarn)
	}
	r.Table([]ui.Column{{}, {Right: true}, {Wrap: true}}, rows, marks)
	if len(inv.Leftovers) > 0 {
		r.Section(i18n.T("mac.h.leftovers"), len(inv.Leftovers))
		rows, marks = nil, nil
		for _, l := range inv.Leftovers {
			rows = append(rows, []string{cut(l.Bundle, 34), macos.HumanSize(l.Size)})
			marks = append(marks, ui.MarkWarn)
		}
		r.Table([]ui.Column{{}, {Right: true}}, rows, marks)
	}
	r.Blank()
	r.Hint(i18n.T("mac.status_next"))
}

func permWhy(x macos.Permission) string {
	if x.Why == macos.WhyNotInstalled {
		return i18n.T("mac.not_installed")
	}
	return i18n.T("mac.perm_gone")
}

func appWhy(inv *macos.Inventory, a macos.AppInfo) string {
	var parts []string
	if !a.LastUsed.IsZero() {
		parts = append(parts, i18n.N("mac.unused", int(inv.Now.Sub(a.LastUsed).Hours()/24)))
	}
	if a.Intel {
		parts = append(parts, i18n.T("mac.intel"))
	}
	if s := a.Source(); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, " · ")
}

// macPermissions offers the privacy entries that point at nothing: binaries
// whose path is gone (preselected) and apps that are no longer installed.
func macPermissions(inv *macos.Inventory, tty bool) error {
	var opts []ui.Option
	changes := map[string]macos.Change{}
	for _, x := range inv.StalePermissions() {
		c := x.ResetChange()
		changes[c.ID] = c
		detail := x.Client
		if x.IsPath {
			detail = strings.Replace(x.Client, macos.Home, "~", 1)
		}
		opts = append(opts, ui.Option{ID: c.ID, Label: x.Label(), Detail: detail, Badge: permWhy(x),
			Risk: verdictRisk(x.Verdict), Selected: x.Verdict == macos.Disable})
	}
	r := ui.NewReport(os.Stdout)
	fmt.Println(r.Muted(i18n.T("mac.perm_summary", len(inv.Permissions), len(opts))))
	return applyChanges(pickChanges(i18n.T("mac.pick.permissions"), opts, changes, tty))
}

// macApps offers the apps not opened in months or still built for Intel, and
// what removed apps left in ~/Library. Nothing is preselected.
func macApps(inv *macos.Inventory, tty bool) error {
	var opts []ui.Option
	changes := map[string]macos.Change{}
	now := time.Now()
	for _, a := range inv.ReviewApps() {
		c := a.UninstallChange(now)
		changes[c.ID] = c
		opts = append(opts, ui.Option{ID: c.ID, Label: c.Title, Detail: appWhy(inv, a), Badge: macos.HumanSize(a.Size), Risk: "dangerous"})
	}
	for _, l := range inv.Leftovers {
		c := l.TrashChange(now)
		changes[c.ID] = c
		detail := i18n.N("mac.leftover_places", len(l.Paths))
		if l.Vendor {
			detail += " · " + i18n.T("mac.leftover_vendor")
		}
		opts = append(opts, ui.Option{ID: c.ID, Label: c.Title, Detail: detail, Badge: macos.HumanSize(l.Size), Risk: "normal"})
	}
	return applyChanges(pickChanges(i18n.T("mac.pick.apps"), opts, changes, tty))
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
		return i18n.N("mac.unused", int(inv.Now.Sub(a.LastUsed).Hours()/24))
	}
	return ""
}

// pickChanges shows a card of changes and returns the chosen ones.
func pickChanges(title string, opts []ui.Option, changes map[string]macos.Change, tty bool) []macos.Change {
	r := ui.NewReport(os.Stdout)
	if len(opts) == 0 {
		macNotice(r, ui.MarkOK, i18n.T("mac.none_found"))
		return nil
	}
	ids, err := ui.Pick(title, i18n.T("mac.pick_hint"), opts, true, tty, os.Stdin, os.Stdout)
	if errors.Is(err, ui.ErrCancelled) || err != nil || len(ids) == 0 {
		macNotice(r, ui.MarkSkip, i18n.T("mac.nothing"))
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
	r := ui.NewReport(os.Stdout)
	for _, c := range changes {
		macChangeHead(r, c.Title, c.Command)
		if c.Sudo() && bridge == nil {
			b, err := sudo.Start(askTerminalPassword, "")
			if err != nil {
				return err
			}
			bridge = b
			runner.Configure(b.Env(), b.PathPrefix())
		}
		res := runner.Run(context.Background(), c.Command, 2*time.Minute, macOutputLine(r))
		if res.ExitCode != 0 {
			failed++
			macFailed(r, res.ExitCode)
			// Part of it may be done: keep the way back.
			if c.KeepsUndoOnFailure() && macos.Record(c) == nil {
				fmt.Println("  " + r.Glyph(ui.MarkInfo) + " " + r.Muted(i18n.T("mac.halfway")))
			}
		} else {
			fmt.Println("  " + r.Glyph(ui.MarkOK))
			if err := macos.Record(c); err != nil {
				fmt.Println("  " + r.Glyph(ui.MarkWarn) + " " + err.Error())
			}
		}
		if c.Manual != "" {
			fmt.Println("  " + r.Glyph(ui.MarkInfo) + " " + c.Manual)
		}
	}
	if len(changes) > 0 {
		r.Blank()
		r.Hint(i18n.T("mac.undo_hint"))
	}
	if failed > 0 {
		return fmt.Errorf("%s", i18n.T("steps.failed_summary", failed))
	}
	return nil
}

// macChangeHead starts a change as omega's other runs start a step: its
// title, then the command on one line.
func macChangeHead(r *ui.Report, title, command string) {
	fmt.Println()
	fmt.Println(r.Accent("▸ ") + r.Bold(title))
	fmt.Println("  " + r.Faint("$ "+r.Clip(command, 4)))
}

// macOutputLine shows a line a change printed, under its command.
func macOutputLine(r *ui.Report) func(string) {
	return func(l string) { fmt.Println("  " + r.Faint("│") + " " + l) }
}

// macFailed says a change failed, with its exit code.
func macFailed(r *ui.Report, code int) {
	fmt.Println("  " + r.Glyph(ui.MarkErr) + " " + r.Err(strings.TrimSpace(strings.TrimPrefix(i18n.T("mac.failed", code), "└"))))
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
		macNotice(ui.NewReport(os.Stdout), ui.MarkInfo, i18n.T("mac.manager", inv.Manager))
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
	r := ui.NewReport(os.Stdout)
	if len(inv.Conflicts) == 0 {
		macNotice(r, ui.MarkOK, i18n.T("mac.no_conflicts"))
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
			macNotice(r, ui.MarkOK, i18n.T("mac.saved_rule", tildePath(macos.PolicyPath())))
		}
	}
	if len(todo) == 0 {
		macNotice(r, ui.MarkSkip, i18n.T("mac.nothing"))
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
			r := ui.NewReport(os.Stdout)
			r.Title("omega mac policy", tildePath(macos.PolicyPath()))
			r.Blank()
			writeBullets(r, i18n.T("mac.policy_defaults"))
			b, err := os.ReadFile(macos.PolicyPath())
			if err != nil {
				r.Blank()
				r.Hint(i18n.T("mac.policy_none"))
				return nil
			}
			r.Blank()
			r.Line(r.Bold(i18n.T("mac.policy_path", tildePath(macos.PolicyPath()))))
			for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
				fmt.Println("    " + l)
			}
			r.Blank()
			r.Hint(i18n.T("mac.policy_edit_hint"))
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
			return editFile(macos.PolicyPath())
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
				writeEmpty(os.Stdout, "omega mac undo", "", i18n.T("mac.undo_empty"), "")
				return nil
			}
			r := ui.NewReport(os.Stdout)
			ids, err := ui.Pick(i18n.T("mac.pick.undo"), i18n.T("mac.pick_hint"), opts, true, term.IsTerminal(int(os.Stdin.Fd())), os.Stdin, os.Stdout)
			if err != nil || len(ids) == 0 {
				macNotice(r, ui.MarkSkip, i18n.T("mac.nothing"))
				return nil
			}
			done := map[time.Time]bool{}
			failed := 0
			// Undoing a change that needed administrator access needs it too,
			// the same way applyChanges gets it.
			var bridge *sudo.Bridge
			defer func() {
				if bridge != nil {
					bridge.Close()
					runner.Configure(nil, "")
				}
			}()
			for _, id := range ids {
				e := entries[id]
				macChangeHead(r, i18n.T("mac.undoing", e.Title), e.Undo)
				if strings.Contains(e.Undo, "sudo ") && bridge == nil {
					b, err := sudo.Start(askTerminalPassword, "")
					if err != nil {
						return err
					}
					bridge = b
					runner.Configure(b.Env(), b.PathPrefix())
				}
				res := runner.Run(context.Background(), e.Undo, 2*time.Minute, macOutputLine(r))
				if res.ExitCode != 0 {
					failed++
					macFailed(r, res.ExitCode)
					continue
				}
				fmt.Println("  " + r.Glyph(ui.MarkOK))
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
		Use: "op <dock-remove|dock-add|perm-reset> <app> [index]", Hidden: true, Args: cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "perm-reset":
				if len(args) != 3 {
					return fmt.Errorf("perm-reset needs a permission and a client")
				}
				return macos.ResetPermission(args[1], args[2])
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

// writeBullets writes a heading line and the "  · " lines under it, each
// wrapped under its own text.
func writeBullets(r *ui.Report, text string) {
	var items []string
	for _, l := range strings.Split(text, "\n") {
		switch t := strings.TrimSpace(l); {
		case strings.HasPrefix(t, "· "):
			items = append(items, strings.TrimPrefix(t, "· "))
		case len(items) > 0 && t != "":
			items[len(items)-1] += " " + t
		case t != "":
			r.Line(r.Bold(t))
		}
	}
	for _, it := range items {
		lines := strings.Split(lipgloss.NewStyle().Width(r.Width()-6).Render(it), "\n")
		for i, l := range lines {
			lead := "    "
			if i == 0 {
				lead = "  " + r.Glyph(ui.MarkOff) + " "
			}
			fmt.Println(lead + strings.TrimRight(l, " "))
		}
	}
}

// offerFullDiskAccess opens the pane where Full Disk Access is granted, when
// someone is at the terminal and says yes: macOS has no dialog for it.
func offerFullDiskAccess() {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return
	}
	if a := strings.ToLower(ask(i18n.T("mac.no_tcc_open")+" [y/N]", "")); a == "" || !strings.ContainsAny(a[:1], "ysojd") {
		return
	}
	r := ui.NewReport(os.Stdout)
	if err := macos.OpenPrivacyPane("SystemPolicyAllFiles"); err != nil {
		macNotice(r, ui.MarkErr, err.Error())
		return
	}
	_, app := macos.HostApp()
	macNotice(r, ui.MarkInfo, i18n.T("mac.no_tcc_opened", app))
}
