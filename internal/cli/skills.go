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
		Short: "Manage the skills of your AI coding agents",
		Long: "Lists the skills installed for Claude Code, Codex, Gemini CLI, opencode, Cursor and GitHub Copilot,\n" +
			"says which agent can see each one, and installs, shares, updates and removes them.",
		Args: cobra.NoArgs,
		RunE: list,
	}
	c.Flags().BoolVar(&asJSON, "json", false, "print the inventory as JSON")

	listCmd := &cobra.Command{Use: "list", Short: "List the installed skills and the agents that see them", Args: cobra.NoArgs, RunE: list}
	listCmd.Flags().BoolVar(&asJSON, "json", false, "print the inventory as JSON")

	audit := &cobra.Command{
		Use:   "audit",
		Short: "Find broken links, invalid SKILL.md files, copies that differ and skills an agent cannot see",
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
		Short: "Install the skills of a folder or a git repository for every agent",
		Long: "Copies the skills found in a folder, a git repository, a GitHub link to a folder or owner/repo.\n" +
			"The files are copied once, to the location most agents read; the others get a link.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return skillInstall(cmd.OutOrStdout(), args[0], opts)
		},
	}
	install.Flags().StringSliceVar(&opts.Only, "skill", nil, "install only this skill of the source (repeatable)")
	install.Flags().StringSliceVar(&opts.To, "to", nil, "install only for these agents: claude, codex, gemini, opencode, cursor, copilot")
	install.Flags().BoolVar(&opts.Copy, "copy", false, "copy to every location instead of linking")
	install.Flags().BoolVar(&opts.Force, "force", false, "replace a skill that is already installed")

	var from string
	var yes bool
	remove := &cobra.Command{
		Use:   "remove <skill>",
		Short: "Remove a skill: links are deleted, folders go to the trash",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return skillRemove(cmd.OutOrStdout(), args[0], from, func(q string) bool { return yes || i18n.IsYes(ask(q, "n")) })
		},
	}
	remove.Flags().StringVar(&from, "from", "", "remove it only from this location: shared, claude, gemini, opencode, cursor, copilot")
	remove.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask")

	var copyAll, dry bool
	sync := &cobra.Command{
		Use:   "sync [skill...]",
		Short: "Make every skill visible to every installed agent",
		Long: "Links each skill into the locations of the agents that cannot see it.\n" +
			"Nothing is overwritten: a skill whose copies differ is reported and left alone.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return skillSync(cmd.OutOrStdout(), args, copyAll, dry)
		},
	}
	sync.Flags().BoolVar(&copyAll, "copy", false, "copy instead of linking")
	sync.Flags().BoolVar(&dry, "dry-run", false, "say what would be done and do nothing")

	var check, force bool
	update := &cobra.Command{
		Use:   "update [skill...]",
		Short: "Update the skills omega installed from where they came from",
		RunE: func(cmd *cobra.Command, args []string) error {
			return skillUpdate(cmd.OutOrStdout(), args, check, force)
		},
	}
	update.Flags().BoolVar(&check, "check", false, "only say which skills have a newer version")
	update.Flags().BoolVar(&force, "force", false, "also replace skills edited on this machine")

	c.AddCommand(listCmd, audit, install, remove, sync, update)
	return c
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
			p += " (broken link)"
		case in.Link != "":
			p += " (link)"
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, ", ")
}

func writeSkillList(w io.Writer, inv *skills.Inventory) {
	fmt.Fprintln(w, "\nAgents")
	for _, a := range inv.Agents {
		var dirs []string
		for _, id := range a.Reads {
			if l, ok := skills.LocationByID(id); ok {
				dirs = append(dirs, l.Path)
			}
		}
		if !a.Installed {
			fmt.Fprintf(w, "  · %-20s not installed\n", a.Name)
			continue
		}
		fmt.Fprintf(w, "  ✓ %-20s %-10s reads %s\n", a.Name, plural(a.Skills, "skill", "skills"), strings.Join(dirs, ", "))
	}
	fmt.Fprintf(w, "\nSkills (%d)\n", len(inv.Skills))
	if len(inv.Skills) == 0 {
		fmt.Fprintln(w, "  None installed. `omega skills install <folder | git-url | owner/repo>` adds one.")
		return
	}
	for _, s := range inv.Skills {
		fmt.Fprintf(w, "\n  %s\n", s.Name)
		if d := s.Description; d != "" {
			if r := []rune(d); len(r) > 110 {
				d = string(r[:109]) + "…"
			}
			fmt.Fprintf(w, "    %s\n", d)
		}
		fmt.Fprintf(w, "    in: %s\n", installPlaces(s))
		if s.Origin != nil {
			fmt.Fprintf(w, "    from: %s\n", s.Origin.Source)
		}
		for _, f := range s.Findings {
			mark := "⚠"
			if f.Level == skills.Note {
				mark = "·"
			}
			fmt.Fprintf(w, "    %s %s\n", mark, f.Text())
		}
	}
	problems, hidden := 0, 0
	for _, s := range inv.Skills {
		problems += s.Problems()
		if len(s.Hidden) > 0 {
			hidden++
		}
	}
	fmt.Fprintln(w)
	if problems > 0 {
		fmt.Fprintf(w, "%s. `omega skills audit` lists them.\n", plural(problems, "problem", "problems"))
	}
	if hidden > 0 {
		fmt.Fprintf(w, "%s not visible to every agent. `omega skills sync` shares them.\n", plural(hidden, "skill is", "skills are"))
	}
}

func writeSkillAudit(w io.Writer, inv *skills.Inventory) {
	problems, notes := 0, 0
	for _, s := range inv.Skills {
		if len(s.Findings) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n%s  (%s)\n", s.Name, installPlaces(s))
		for _, f := range s.Findings {
			if f.Level == skills.Problem {
				problems++
				fmt.Fprintf(w, "  ⚠ %s\n", f.Text())
			} else {
				notes++
				fmt.Fprintf(w, "  · %s\n", f.Text())
			}
		}
	}
	if problems+notes == 0 {
		fmt.Fprintf(w, "✓ %s checked, nothing to report.\n", plural(len(inv.Skills), "skill", "skills"))
		return
	}
	fmt.Fprintf(w, "\n%s checked: %s, %s.\n", plural(len(inv.Skills), "skill", "skills"),
		plural(problems, "problem", "problems"), plural(notes, "note", "notes"))
}

func skillInstall(w io.Writer, source string, opts skills.InstallOptions) error {
	done, err := skills.InstallFrom(context.Background(), source, opts)
	for _, d := range done {
		fmt.Fprintf(w, "✓ %s\n", d.Skill)
		for _, p := range d.Paths {
			fmt.Fprintf(w, "    %s\n", skills.Tilde(p))
		}
		for _, p := range d.Skipped {
			fmt.Fprintf(w, "    ⚠ %s left alone: a link to a folder outside the skills locations\n", skills.Tilde(p))
		}
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "\n%s installed. Agents already running see them after a restart.\n", plural(len(done), "skill", "skills"))
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
		fmt.Fprintf(w, "⚠ %s: %s\n", s.Skill, s.Reason)
	}
	if len(actions) == 0 {
		if len(skipped) == 0 {
			fmt.Fprintln(w, "✓ Every installed agent already sees every skill.")
		}
		return nil
	}
	var failed int
	for _, a := range actions {
		if dry {
			fmt.Fprintf(w, "  would link %s → %s\n", skills.Tilde(a.Path), skills.Tilde(a.Source))
			continue
		}
		linked, err := skills.Apply(a, copyAll)
		switch {
		case err != nil:
			failed++
			fmt.Fprintf(w, "⚠ %s: %v\n", a.Skill, err)
		case linked:
			fmt.Fprintf(w, "✓ %s → %s\n", skills.Tilde(a.Path), skills.Tilde(a.Source))
		default:
			fmt.Fprintf(w, "✓ %s (copy of %s)\n", skills.Tilde(a.Path), skills.Tilde(a.Source))
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d could not be shared", failed, len(actions))
	}
	return nil
}

func skillUpdate(w io.Writer, names []string, check, force bool) error {
	results, err := skills.Update(context.Background(), names, check, force)
	marks := map[string]string{skills.Updated: "✓", skills.Current: "·", skills.Available: "↑",
		skills.Modified: "⚠", skills.Failed: "⚠", skills.Untracked: "·"}
	words := map[string]string{skills.Updated: "updated", skills.Current: "up to date", skills.Available: "a newer version is available",
		skills.Modified: "not updated", skills.Failed: "failed", skills.Untracked: "not tracked"}
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
		fmt.Fprintln(w, "No skill was installed with omega, so there is nothing to update.")
	}
	if failed > 0 {
		return fmt.Errorf("%s could not be updated", plural(failed, "skill", "skills"))
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
	fmt.Fprintf(w, "Removing %s:\n", name)
	for _, in := range installs {
		kind := "folder, to the trash"
		if in.Link != "" {
			kind = "link"
		}
		fmt.Fprintf(w, "  %s  (%s)\n", skills.Tilde(in.Path), kind)
	}
	if !confirm("Remove it?") {
		fmt.Fprintln(w, "Nothing was removed.")
		return nil
	}
	kept, err := skills.Remove(name, installs)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "✓ %s removed.\n", name)
	for _, k := range kept {
		fmt.Fprintf(w, "  The folder is in %s\n", skills.Tilde(k))
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
