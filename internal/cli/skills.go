package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/skills"
	"github.com/lcajigasm/omega/internal/ui"
)

// The skills of the AI coding agents on this machine: what is installed,
// who can see it, and installing, sharing, updating and removing them. The
// desktop app shows the same through the functions at the end of the file.

func skillsCmd() *cobra.Command {
	var asJSON bool
	list := func(cmd *cobra.Command, _ []string) error {
		inv, err := skills.Scan()
		if err != nil {
			return err
		}
		if asJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(inv)
		}
		writeSkillList(cmd.OutOrStdout(), inv)
		return nil
	}
	c := &cobra.Command{
		Use:   "skills",
		Short: i18n.T("cmd.skills"),
		Long:  i18n.T("cmd.skills_long"),
		Example: "  omega skills\n" +
			"  omega skills install anthropics/skills --skill pdf\n" +
			"  omega skills audit\n" +
			"  omega skills sync --dry-run\n" +
			"  omega skills update --check\n" +
			"  omega skills remove pdf --from cursor",
		Args: cobra.NoArgs,
		RunE: list,
	}
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.tools_json"))

	listCmd := &cobra.Command{Use: "list", Short: i18n.T("cmd.skills_list"), Args: cobra.NoArgs, RunE: list}
	listCmd.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.tools_json"))

	audit := &cobra.Command{
		Use:   "audit",
		Short: i18n.T("cmd.skills_audit"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			inv, err := skills.Scan()
			if err != nil {
				return err
			}
			writeSkillAudit(cmd.OutOrStdout(), inv)
			return nil
		},
	}

	var opts skills.InstallOptions
	install := &cobra.Command{
		Use:   "install <folder | git-url | owner/repo>",
		Short: i18n.T("cmd.skills_install"),
		Long:  i18n.T("cmd.skills_install_long"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return skillInstall(cmd.OutOrStdout(), args[0], opts)
		},
	}
	install.Flags().StringSliceVar(&opts.Only, "skill", nil, i18n.T("flag.skills_skill"))
	install.Flags().StringSliceVar(&opts.To, "to", nil, i18n.T("flag.skills_to"))
	install.Flags().BoolVar(&opts.Copy, "copy", false, i18n.T("flag.skills_copy_all"))
	install.Flags().BoolVar(&opts.Force, "force", false, i18n.T("flag.skills_force_install"))

	var from string
	var yes bool
	remove := &cobra.Command{
		Use:   "remove <skill>",
		Short: i18n.T("cmd.skills_remove"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return skillRemove(cmd.OutOrStdout(), args[0], from, func(q string) bool { return yes || i18n.IsYes(ask(q, "n")) })
		},
	}
	remove.Flags().StringVar(&from, "from", "", i18n.T("flag.skills_from"))
	remove.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("flag.tools_yes"))

	var copyAll, dry bool
	sync := &cobra.Command{
		Use:   "sync [skill...]",
		Short: i18n.T("cmd.skills_sync"),
		Long:  i18n.T("cmd.skills_sync_long"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return skillSync(cmd.OutOrStdout(), args, copyAll, dry)
		},
	}
	sync.Flags().BoolVar(&copyAll, "copy", false, i18n.T("flag.skills_copy"))
	sync.Flags().BoolVar(&dry, "dry-run", false, i18n.T("flag.tools_dry"))

	var check, force bool
	update := &cobra.Command{
		Use:   "update [skill...]",
		Short: i18n.T("cmd.skills_update"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return skillUpdate(cmd.OutOrStdout(), args, check, force)
		},
	}
	update.Flags().BoolVar(&check, "check", false, i18n.T("flag.skills_check"))
	update.Flags().BoolVar(&force, "force", false, i18n.T("flag.skills_force_update"))

	c.AddCommand(listCmd, audit, install, remove, sync, update)
	return c
}

// skillCount and problemCount are "1 skill", "3 skills", translated.
func skillCount(n int) string {
	return plural(n, i18n.T("skills.w_skill"), i18n.T("skills.w_skills"))
}

func problemCount(n int) string {
	return plural(n, i18n.T("skills.w_problem"), i18n.T("skills.w_problems"))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func agentNames(ids []string) string {
	var names []string
	for _, id := range ids {
		if a, ok := skills.AgentByID(id); ok {
			names = append(names, a.Name)
		}
	}
	return strings.Join(names, ", ")
}

// installPlaces says where a skill is: "shared, claude (link)".
func installPlaces(s skills.Skill) string {
	var parts []string
	for _, in := range s.Installs {
		p := in.Location
		switch {
		case in.Broken:
			p += " (" + i18n.T("skills.broken_link") + ")"
		case in.Link != "":
			p += " (" + i18n.T("skills.link") + ")"
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, ", ")
}

func writeSkillList(w io.Writer, inv *skills.Inventory) {
	r := ui.NewReport(w)
	r.Title("omega skills", skillCount(len(inv.Skills)))
	r.Section(i18n.T("skills.agents"), -1)
	var rows [][]string
	var marks []ui.Mark
	for _, a := range inv.Agents {
		var dirs []string
		for _, id := range a.Reads {
			if l, ok := skills.LocationByID(id); ok {
				dirs = append(dirs, l.Path)
			}
		}
		if !a.Installed {
			rows = append(rows, []string{a.Name, r.Faint(i18n.T("skills.not_installed")), ""})
			marks = append(marks, ui.MarkOff)
			continue
		}
		rows = append(rows, []string{a.Name, skillCount(a.Skills), r.Muted(i18n.T("skills.reads", strings.Join(dirs, ", ")))})
		marks = append(marks, ui.MarkOK)
	}
	r.Table([]ui.Column{{}, {}, {Wrap: true}}, rows, marks)
	r.Section(i18n.T("skills.skills"), len(inv.Skills))
	if len(inv.Skills) == 0 {
		r.Line(r.Glyph(ui.MarkOff) + " " + i18n.T("skills.none"))
		r.Blank()
		return
	}
	for n, s := range inv.Skills {
		if n > 0 {
			r.Blank()
		}
		mark := ui.MarkOK
		if s.Problems() > 0 {
			mark = ui.MarkWarn
		}
		r.Check(mark, s.Name, 0, "")
		if d := s.Description; d != "" {
			r.Detail(r.Clip(d, 4))
		}
		r.Detail(i18n.T("skills.in", installPlaces(s)))
		if s.Origin != nil {
			// A source is a path or a URL: whole, so it can be copied.
			fmt.Fprintln(w, "    "+r.Muted(i18n.T("skills.from", s.Origin.Source)))
		}
		for _, f := range s.Findings {
			writeFinding(w, r, f)
		}
	}
	problems, hidden := 0, 0
	for _, s := range inv.Skills {
		problems += s.Problems()
		if len(s.Hidden) > 0 {
			hidden++
		}
	}
	r.Blank()
	if problems > 0 {
		r.Hint(i18n.T("skills.problems_hint", problemCount(problems)))
	}
	if hidden > 0 {
		if hidden == 1 {
			r.Hint(i18n.T("skills.hidden_one"))
		} else {
			r.Hint(i18n.T("skills.hidden_many", hidden))
		}
	}
}

// writeFinding is one finding under its skill: a problem marked "!", a
// note marked "·".
func writeFinding(w io.Writer, r *ui.Report, f skills.Finding) {
	mark := ui.MarkWarn
	if f.Level == skills.Note {
		mark = ui.MarkOff
	}
	fmt.Fprintln(w, "    "+r.Glyph(mark)+" "+f.Text())
}

func writeSkillAudit(w io.Writer, inv *skills.Inventory) {
	r := ui.NewReport(w)
	r.Title("omega skills audit", skillCount(len(inv.Skills)))
	problems, notes := 0, 0
	for _, s := range inv.Skills {
		if len(s.Findings) == 0 {
			continue
		}
		mark := ui.MarkOff
		if s.Problems() > 0 {
			mark = ui.MarkWarn
		}
		r.Blank()
		r.Check(mark, s.Name, 0, r.Muted(installPlaces(s)))
		for _, f := range s.Findings {
			if f.Level == skills.Problem {
				problems++
			} else {
				notes++
			}
			writeFinding(w, r, f)
		}
	}
	r.Blank()
	if problems+notes == 0 {
		r.Line(r.Glyph(ui.MarkOK) + " " + i18n.T("skills.audit_clean", skillCount(len(inv.Skills))))
		return
	}
	mark := ui.MarkOff
	if problems > 0 {
		mark = ui.MarkWarn
	}
	fmt.Fprintln(w, r.Glyph(mark)+" "+i18n.T("skills.audit_summary", skillCount(len(inv.Skills)), problemCount(problems),
		plural(notes, i18n.T("skills.w_note"), i18n.T("skills.w_notes"))))
}

func skillInstall(w io.Writer, source string, opts skills.InstallOptions) error {
	done, err := skills.InstallFrom(context.Background(), source, opts)
	for _, d := range done {
		say(w, ui.MarkOK, d.Skill)
		for _, p := range d.Paths {
			fmt.Fprintf(w, "    %s\n", skills.Tilde(p))
		}
		for _, p := range d.Skipped {
			fmt.Fprintf(w, "    ! %s\n", i18n.T("skills.left_alone", skills.Tilde(p)))
		}
	}
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "\n"+i18n.T("skills.installed", skillCount(len(done))))
	return nil
}

func skillSync(w io.Writer, names []string, copyAll, dry bool) error {
	inv, err := skills.Scan()
	if err != nil {
		return err
	}
	actions, skipped, err := skills.PlanSync(inv, names)
	if err != nil {
		return err
	}
	for _, s := range skipped {
		fmt.Fprintf(w, "! %s: %s\n", s.Skill, s.Reason)
	}
	if len(actions) == 0 {
		if len(skipped) == 0 {
			say(w, ui.MarkOK, i18n.T("skills.all_visible"))
		}
		return nil
	}
	var failed int
	for _, a := range actions {
		if dry {
			fmt.Fprintln(w, "› "+i18n.T("skills.would_link", skills.Tilde(a.Path), skills.Tilde(a.Source)))
			continue
		}
		linked, err := skills.Apply(a, copyAll)
		switch {
		case err != nil:
			failed++
			fmt.Fprintf(w, "✗ %s: %v\n", a.Skill, err)
		case linked:
			fmt.Fprintf(w, "✓ %s → %s\n", skills.Tilde(a.Path), skills.Tilde(a.Source))
		default:
			say(w, ui.MarkOK, i18n.T("skills.copy_of", skills.Tilde(a.Path), skills.Tilde(a.Source)))
		}
	}
	if failed > 0 {
		return fmt.Errorf("%s", i18n.T("skills.sync_failed", failed, len(actions)))
	}
	return nil
}

func skillUpdate(w io.Writer, names []string, check, force bool) error {
	results, err := skills.Update(context.Background(), names, check, force)
	marks := map[string]string{skills.Updated: "✓", skills.Current: "·", skills.Available: "↑",
		skills.Modified: "!", skills.Failed: "✗", skills.Untracked: "·"}
	words := map[string]string{skills.Updated: i18n.T("skills.st_updated"), skills.Current: i18n.T("skills.st_current"),
		skills.Available: i18n.T("skills.st_available"), skills.Modified: i18n.T("skills.st_modified"),
		skills.Failed: i18n.T("skills.st_failed"), skills.Untracked: i18n.T("skills.st_untracked")}
	failed := 0
	for _, r := range results {
		line := fmt.Sprintf("%s %s: %s", marks[r.State], r.Skill, words[r.State])
		if r.Detail != "" {
			line += " (" + r.Detail + ")"
		}
		fmt.Fprintln(w, line)
		if r.State == skills.Failed {
			failed++
		}
	}
	if err != nil {
		return err
	}
	if len(results) == 0 {
		fmt.Fprintln(w, i18n.T("skills.nothing_to_update"))
	}
	if failed > 0 {
		return fmt.Errorf("%s", i18n.T("skills.update_failed", skillCount(failed)))
	}
	return nil
}

// skillRemove shows what goes and removes it once confirm says yes.
func skillRemove(w io.Writer, name, from string, confirm func(question string) bool) error {
	inv, err := skills.Scan()
	if err != nil {
		return err
	}
	installs, err := skills.PlanRemove(inv, name, from)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, i18n.T("skills.removing", name))
	for _, in := range installs {
		kind := i18n.T("skills.folder_trash")
		if in.Link != "" {
			kind = i18n.T("skills.link")
		}
		fmt.Fprintf(w, "  · %s  (%s)\n", skills.Tilde(in.Path), kind)
	}
	if !confirm(i18n.T("skills.ask_remove")) {
		fmt.Fprintln(w, "– "+i18n.T("skills.not_removed"))
		return nil
	}
	kept, err := skills.Remove(name, installs)
	if err != nil {
		return err
	}
	say(w, ui.MarkOK, i18n.T("skills.removed", name))
	for _, k := range kept {
		fmt.Fprintln(w, "  "+i18n.T("skills.folder_in", skills.Tilde(k)))
	}
	return nil
}

// ---- the desktop app ----

// SkillPlace is one installation of a skill, as the app shows it.
type SkillPlace struct {
	Location string `json:"location"`
	Path     string `json:"path"`
	Link     bool   `json:"link"`
	Broken   bool   `json:"broken"`
}

// SkillNote is a finding of the audit, in the interface language.
type SkillNote struct {
	Problem bool   `json:"problem"`
	Text    string `json:"text"`
}

// SkillInfo is a skill as the app shows it.
type SkillInfo struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Places      []SkillPlace `json:"places"`
	Visible     []string     `json:"visible"` // names of the agents that see it
	Hidden      []string     `json:"hidden"`
	Source      string       `json:"source"` // where omega installed it from; "" when it did not
	Notes       []SkillNote  `json:"notes"`
	Problems    int          `json:"problems"`
}

// SkillAgent is an agent as the app shows it.
type SkillAgent struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Installed bool     `json:"installed"`
	Skills    int      `json:"skills"`
	Dirs      []string `json:"dirs"`
}

// SkillsView is the skills screen of the app.
type SkillsView struct {
	Agents []SkillAgent `json:"agents"`
	Skills []SkillInfo  `json:"skills"`
}

// SkillList returns the agents and the skills of this machine.
func SkillList() (SkillsView, error) {
	v := SkillsView{Agents: []SkillAgent{}, Skills: []SkillInfo{}}
	inv, err := skills.Scan()
	if err != nil {
		return v, err
	}
	names := func(ids []string) []string {
		out := []string{}
		for _, id := range ids {
			if a, ok := skills.AgentByID(id); ok {
				out = append(out, a.Name)
			}
		}
		return out
	}
	for _, a := range inv.Agents {
		sa := SkillAgent{ID: a.ID, Name: a.Name, Installed: a.Installed, Skills: a.Skills, Dirs: []string{}}
		for _, id := range a.Reads {
			if l, ok := skills.LocationByID(id); ok {
				sa.Dirs = append(sa.Dirs, l.Path)
			}
		}
		v.Agents = append(v.Agents, sa)
	}
	for _, s := range inv.Skills {
		in := SkillInfo{Name: s.Name, Description: s.Description, Places: []SkillPlace{}, Visible: names(s.Visible),
			Hidden: names(s.Hidden), Notes: []SkillNote{}, Problems: s.Problems()}
		if s.Origin != nil {
			in.Source = s.Origin.Source
		}
		for _, p := range s.Installs {
			in.Places = append(in.Places, SkillPlace{Location: p.Location, Path: skills.Tilde(p.Path), Link: p.Link != "", Broken: p.Broken})
		}
		for _, f := range s.Findings {
			text := i18n.T("gui.skills.f." + f.Code)
			if f.Detail != "" {
				text += ": " + f.Detail
			}
			in.Notes = append(in.Notes, SkillNote{Problem: f.Level == skills.Problem, Text: text})
		}
		v.Skills = append(v.Skills, in)
	}
	return v, nil
}

// SkillAction does what the app asks and returns what the command would
// have printed: "install" (arg is the source), "sync" and "update" (name
// may be empty for all), "remove" (arg is the location, or empty for all).
// The app asks before it removes.
func SkillAction(action, name, arg string) (string, error) {
	var out bytes.Buffer
	var err error
	var names []string
	if name != "" {
		names = []string{name}
	}
	switch action {
	case "install":
		err = skillInstall(&out, arg, skills.InstallOptions{})
	case "sync":
		err = skillSync(&out, names, false, false)
	case "update":
		err = skillUpdate(&out, names, false, false)
	case "remove":
		err = skillRemove(&out, name, arg, func(string) bool { return true })
	default:
		err = fmt.Errorf("unknown action %q", action)
	}
	return strings.TrimRight(out.String(), "\n"), err
}
