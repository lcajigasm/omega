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
	"github.com/lcajigasm/omega/internal/steps"
	"github.com/lcajigasm/omega/internal/ui"
)

// taskOrder is the order the maintenance tasks are listed in.
var taskOrder = []struct{ task, detail string }{
	{"", "slash.task_full"},
	{agent.TaskUpdate, "slash.task_update"},
	{agent.TaskClean, "slash.task_clean"},
	{agent.TaskAnalyze, "slash.task_analyze"},
	{agent.TaskHealth, "slash.task_health"},
	{agent.TaskTune, "slash.task_tune"},
	{agent.TaskFix, "slash.task_fix"},
}

// runOptions are the other ways to start a run with the agent, offered with
// the right arrow on every entry that starts one.
var runOptions = []struct{ flag, detail string }{
	{"--dry-run", "flag.dry_run"},
	{"--auto", "flag.auto"},
	{"--yes", "flag.yes"},
	{"--sudo", "flag.sudo"},
	{"--no-tui", "flag.no_tui"},
}

// settingValues are the values `omega config set` accepts for each key; a
// key with none is typed.
var settingValues = []struct {
	key    string
	values []string
}{
	{"language", []string{"auto", "en", "es", "fr", "de", "pt"}},
	{"approval", []string{config.ApproveDangerous, config.ApproveAll, config.ApproveNever}},
	{"effort", []string{"low", "medium", "high", "xhigh", "max"}},
	{"model_catalog", []string{config.CatalogLive, config.CatalogOff}},
	{"provider", nil}, // filled with the providers
	{"model", nil},
	{"command_timeout", nil},
	{"skip_steps", nil},
}

func cmdLabel(args ...string) string { return strings.TrimSpace("omega " + strings.Join(args, " ")) }

// withRunOptions adds the run variants to an entry that starts the agent.
func withRunOptions(it ui.MenuItem) ui.MenuItem {
	for _, o := range runOptions {
		if o.flag == "--sudo" && goos == "windows" {
			continue // Windows has no sudo
		}
		args := append(append([]string(nil), it.Args...), o.flag)
		it.Options = append(it.Options, ui.MenuItem{Label: cmdLabel(args...), Detail: firstLine(i18n.T(o.detail)), Args: args})
	}
	return it
}

// agentCommands are the commands that start a session with the agent, and so
// take the run options.
var agentCommands = map[string]bool{"full": true, "mac": true, "dev": true}

// menuFromCommand turns a command and everything under it into menu rows.
// It is built from the command tree, so a new subcommand shows up here
// without anyone remembering to add it.
func menuFromCommand(c *cobra.Command, group string) ui.MenuItem {
	path := strings.Fields(strings.TrimPrefix(c.CommandPath(), "omega"))
	it := ui.MenuItem{Group: group, Label: cmdLabel(path...), Detail: firstLine(c.Short), Args: path}
	if agentCommands[strings.Join(path, " ")] {
		it = withRunOptions(it)
	}
	switch strings.Join(path, " ") {
	case "config set":
		it.Children = settingRows()
		return it
	case "auth set", "auth remove":
		for _, p := range llm.Providers {
			args := append(append([]string(nil), path...), p.Name)
			it.Children = append(it.Children, ui.MenuItem{Label: cmdLabel(args...), Detail: p.Label, Args: args})
		}
		return it
	case "steps run":
		for _, st := range steps.All {
			if !st.Available() {
				continue
			}
			args := append(append([]string(nil), path...), st.ID)
			it.Children = append(it.Children, ui.MenuItem{Group: string(st.Category), Label: cmdLabel(args...), Detail: st.Name, Args: args})
		}
		return it
	case "model":
		it.Children = []ui.MenuItem{
			{Label: cmdLabel("model"), Detail: firstLine(c.Short), Args: []string{"model"}},
			{Label: cmdLabel("model", "auto"), Detail: i18n.T("model.auto"), Args: []string{"model", "auto"}},
			{Label: cmdLabel("model", "…"), Detail: i18n.T("menu.type_model"), Args: []string{"model"}, Ask: i18n.T("ui.label_model")},
		}
		return it
	case "models":
		it.Children = []ui.MenuItem{
			{Label: cmdLabel("models"), Detail: firstLine(c.Short), Args: []string{"models"}},
			{Label: cmdLabel("models", "--refresh"), Detail: firstLine(i18n.T("flag.refresh")), Args: []string{"models", "--refresh"}},
		}
		for _, p := range llm.Providers {
			if !p.Local {
				it.Children = append(it.Children, ui.MenuItem{Label: cmdLabel("models", p.Name), Detail: p.Label, Args: []string{"models", p.Name}})
			}
		}
		return it
	case "dev install":
		it.Children = []ui.MenuItem{
			{Label: cmdLabel("dev", "install"), Detail: firstLine(c.Short), Args: path},
			{Label: cmdLabel("dev", "install", "…"), Detail: i18n.T("menu.type_tools"), Args: path, Ask: i18n.T("menu.tools")},
		}
		return it
	case "logs view":
		it.Children = []ui.MenuItem{
			{Label: cmdLabel("logs", "view"), Detail: firstLine(c.Short), Args: path},
			{Label: cmdLabel("logs", "view", "…"), Detail: i18n.T("menu.type_run"), Args: path, Ask: "n"},
		}
		return it
	}
	var subs []*cobra.Command
	for _, sub := range c.Commands() {
		if !sub.Hidden && sub.Name() != "help" {
			subs = append(subs, sub)
		}
	}
	if len(subs) == 0 {
		return it
	}
	if c.Runnable() {
		self := it
		self.Group, self.Children = "", nil
		it.Children = append(it.Children, self)
	}
	for _, sub := range subs {
		it.Children = append(it.Children, menuFromCommand(sub, ""))
	}
	it.Options = nil
	return it
}

// settingRows lists every setting with the values it can take.
func settingRows() []ui.MenuItem {
	var rows []ui.MenuItem
	for _, sv := range settingValues {
		base := []string{"config", "set", sv.key}
		row := ui.MenuItem{Label: cmdLabel(base...), Detail: i18n.T("menu.set." + sv.key), Args: base}
		values := sv.values
		if sv.key == "provider" {
			for _, p := range llm.Providers {
				values = append(values, p.Name)
			}
		}
		if len(values) == 0 {
			row.Label, row.Ask = cmdLabel(append(base, "…")...), sv.key
			rows = append(rows, row)
			continue
		}
		for _, v := range values {
			args := append(append([]string(nil), base...), v)
			row.Children = append(row.Children, ui.MenuItem{Label: cmdLabel(args...), Args: args})
		}
		rows = append(rows, row)
	}
	return rows
}

// menuGroups says which commands sit under which heading, in order. A
// command missing from here is still listed, under the last heading.
var menuGroups = []struct {
	group    string
	commands []string
}{
	{"maintain", []string{"docker"}},
	{"mac", []string{"mac"}},
	{"dev", []string{"dev"}},
	{"models", []string{"setup", "model", "models", "auth"}},
	{"state", []string{"doctor", "steps", "logs", "notes"}},
	{"settings", []string{"config"}},
}

// menuTree is the whole home screen: the maintenance tasks, a way to ask in
// words, and every command of the tree with what hangs from it.
func menuTree(root *cobra.Command) []ui.MenuItem {
	var items []ui.MenuItem
	maintain := i18n.T("menu.g.maintain")
	// The guided run comes first: it is the one that explains the others.
	for _, c := range root.Commands() {
		if c.Name() == "full" {
			items = append(items, menuFromCommand(c, maintain))
		}
	}
	for _, t := range taskOrder {
		var args []string
		if t.task != "" {
			args = []string{t.task}
		}
		items = append(items, withRunOptions(ui.MenuItem{Group: maintain, Label: cmdLabel(args...), Detail: firstLine(i18n.T(t.detail)), Args: args}))
	}
	byName := map[string]*cobra.Command{}
	for _, c := range root.Commands() {
		if !c.Hidden && c.Name() != "help" && c.Name() != "completion" && c.Name() != "menu" && c.Name() != "full" {
			byName[c.Name()] = c
		}
	}
	placed := map[string]bool{}
	for _, g := range menuGroups {
		if g.group == "mac" && goos != "darwin" {
			placed["mac"] = true
			continue
		}
		for _, name := range g.commands {
			if c, ok := byName[name]; ok {
				items = append(items, menuFromCommand(c, i18n.T("menu.g."+g.group)))
				placed[name] = true
			}
		}
		if g.group == "maintain" {
			items = append(items, ui.MenuItem{Group: maintain, Label: `omega -p "…"`, Detail: i18n.T("menu.ask_words"),
				Args: []string{"-p"}, Ask: i18n.T("menu.request")})
		}
	}
	last := i18n.T("menu.g.settings")
	for _, c := range root.Commands() {
		if _, ok := byName[c.Name()]; ok && !placed[c.Name()] {
			items = append(items, menuFromCommand(c, last))
		}
	}
	items = append(items,
		ui.MenuItem{Group: last, Label: "omega --help", Detail: i18n.T("menu.help"), Args: []string{"--help"}},
		ui.MenuItem{Group: last, Label: "omega --version", Detail: i18n.T("menu.version"), Args: []string{"--version"}})
	return items
}

// fullScreen reports whether a command line opens the full-screen
// interface: a task, a request in words, the guided run or one of the
// reviews, unless --no-tui asks for lines.
func fullScreen(args []string) bool {
	var words []string
	for _, a := range args {
		switch {
		case a == "--no-tui" || a == "--help" || a == "--version" || a == "-h":
			return false
		case a == "-p" || a == "--prompt":
			return true
		case !strings.HasPrefix(a, "-"):
			words = append(words, a)
		}
	}
	switch len(words) {
	case 0:
		return true
	case 1:
		if _, ok := taskFor(words[0]); ok {
			return true
		}
		return words[0] == "full" || words[0] == "mac" || words[0] == "dev"
	}
	return false
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
			in := bufio.NewReader(os.Stdin)
			for {
				// Built on every pass: a step installed or a key saved from
				// the menu changes what it offers.
				picked, ok, err := ui.RunMenu(menuInfo(), menuTree(cmd.Root()))
				if err != nil || !ok {
					return err
				}
				c := exec.Command(exe, picked...)
				c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
				err = c.Run()
				if err != nil {
					if _, exited := err.(*exec.ExitError); !exited {
						return err
					}
				}
				// A session that had the whole screen already ended on its own
				// last screen: asking for Enter again is one prompt too many.
				// What printed lines, or failed, stays until it has been read.
				if err == nil && fullScreen(picked) {
					continue
				}
				fmt.Print("\n" + i18n.T("menu.back"))
				if _, err := in.ReadString('\n'); err != nil {
					return nil
				}
			}
		},
	}
}
