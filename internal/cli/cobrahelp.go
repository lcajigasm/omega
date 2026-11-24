package cli

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/lcajigasm/omega/internal/i18n"
)

// What cobra and pflag would print in English, said in the user's language:
// the help templates, the help flag and command, and the errors of a
// command line that does not parse. Every such error ends with the help to
// read and exits with 64 (EX_USAGE), so a script can tell a typing mistake
// from a run that failed.

// exitUsage is the exit status of a command line omega cannot use.
const exitUsage = 64

// usageError is a command line omega cannot use: a word, a flag or a number
// of arguments that is wrong. It says what to read next.
type usageError struct {
	msg  string
	path string // the command whose help explains it
}

func (e usageError) Error() string {
	if e.path == "" {
		return e.msg
	}
	return e.msg + "\n" + i18n.T("err.see_help", e.path+" --help")
}

func (e usageError) ExitCode() int { return exitUsage }

// usageErr makes err a usage error of cmd.
func usageErr(cmd *cobra.Command, msg string) error {
	path := "omega"
	if cmd != nil {
		path = cmd.CommandPath()
	}
	return usageError{msg: msg, path: path}
}

// Flag groups: the flags of an agent run and those that only mean something
// without the AI are shown apart in the help.
const (
	flagGroup = "omega_group"
	groupAI   = "agent"
	groupNoAI = "noai"
)

func markGroup(fs *pflag.FlagSet, group string, names ...string) {
	for _, n := range names {
		_ = fs.SetAnnotation(n, flagGroup, []string{group})
	}
}

func groupOf(f *pflag.Flag) string {
	if g := f.Annotations[flagGroup]; len(g) > 0 {
		return g[0]
	}
	return ""
}

// flagSections renders a command's own flags by group, each under its
// heading, and the flags it inherits.
func flagSections(c *cobra.Command) string {
	var b strings.Builder
	section := func(title string, fs *pflag.FlagSet) {
		if fs.HasAvailableFlags() {
			fmt.Fprintf(&b, "\n\n%s:\n%s", title, strings.TrimRight(fs.FlagUsages(), " \n"))
		}
	}
	plain := pflag.NewFlagSet("plain", pflag.ContinueOnError)
	agentSet := pflag.NewFlagSet("agent", pflag.ContinueOnError)
	noAI := pflag.NewFlagSet("noai", pflag.ContinueOnError)
	c.LocalFlags().VisitAll(func(f *pflag.Flag) {
		switch groupOf(f) {
		case groupAI:
			agentSet.AddFlag(f)
		case groupNoAI:
			noAI.AddFlag(f)
		default:
			plain.AddFlag(f)
		}
	})
	section(i18n.T("help.flags"), plain)
	section(i18n.T("help.flags_agent"), agentSet)
	section(i18n.T("help.flags_noai"), noAI)
	section(i18n.T("help.flags_global"), c.InheritedFlags())
	return b.String()
}

// seeAlso is the "See also" line of a command, from its annotation.
func seeAlso(c *cobra.Command) string {
	if s := c.Annotations["see_also"]; s != "" {
		return "\n\n" + i18n.T("help.see_also") + ": " + s
	}
	return ""
}

func init() {
	cobra.AddTemplateFunc("omegaFlags", flagSections)
	cobra.AddTemplateFunc("omegaSeeAlso", seeAlso)
	cobra.AddTemplateFunc("omegaUseLine", func(c *cobra.Command) string {
		return strings.Replace(c.UseLine(), " [flags]", " "+i18n.T("help.flags_arg"), 1)
	})
}

// usageTemplate is cobra's usage template with its words translated and the
// flags in groups.
func usageTemplate() string {
	return i18n.T("help.usage") + `:{{if .Runnable}}
  {{omegaUseLine .}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} ` + i18n.T("help.command_arg") + `{{end}}{{if gt (len .Aliases) 0}}

` + i18n.T("help.aliases") + `:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

` + i18n.T("help.examples") + `:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

` + i18n.T("help.commands") + `:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

` + i18n.T("help.more_commands") + `:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{omegaFlags .}}{{omegaSeeAlso .}}{{if .HasAvailableSubCommands}}

` + i18n.T("help.more", "{{.CommandPath}} "+i18n.T("help.command_arg")+" --help") + `{{end}}
`
}

// finishTree gives every command of the tree what cobra would otherwise
// add in English: the help flag, the help command, the usage template and
// the errors of a command line that does not parse.
func finishTree(root *cobra.Command) {
	root.SetUsageTemplate(usageTemplate() + "\n" + i18n.T("help.ask") + "\n")
	root.SetFlagErrorFunc(flagError)
	root.SetHelpCommand(helpCmd(root))
	if root.Version != "" && root.Flags().Lookup("version") == nil {
		root.Flags().Bool("version", false, i18n.T("flag.version"))
	}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Flags().Lookup("help") == nil {
			c.Flags().BoolP("help", "h", false, i18n.T("flag.help", c.CommandPath()))
		}
		if c.Args != nil {
			c.Args = translatedArgs(c.Args)
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}

// helpCmd is `omega help [command]`. A topic that is not a command is an
// error with the closest names, not the root help and a status of 0.
func helpCmd(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:     "help [command]",
		Short:   i18n.T("cmd.help"),
		GroupID: "help",
		ValidArgsFunction: func(c *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			parent, _, err := root.Find(args)
			if err != nil || parent == nil {
				parent = root
			}
			var out []cobra.Completion
			for _, sub := range parent.Commands() {
				if sub.IsAvailableCommand() && strings.HasPrefix(sub.Name(), toComplete) {
					out = append(out, cobra.CompletionWithDesc(sub.Name(), sub.Short))
				}
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(c *cobra.Command, args []string) error {
			cmd, rest, err := root.Find(args)
			if err != nil || cmd == nil || len(rest) > 0 {
				parent := cmd
				if parent == nil {
					parent = root
				}
				word := strings.Join(args, " ")
				if len(rest) > 0 {
					word = rest[0]
				}
				return unknownWord(parent, word)
			}
			cmd.InitDefaultHelpFlag()
			return cmd.Help()
		},
	}
}

// unknownWord is the error for a word that is not a subcommand of parent:
// the closest subcommands when there are some.
func unknownWord(parent *cobra.Command, word string) error {
	msg := i18n.T("err.unknown_sub", word, parent.CommandPath())
	if s := suggestSub(parent, word); len(s) > 0 {
		msg += " " + i18n.T("err.did_you_mean", strings.Join(s, ", "))
	}
	return usageErr(parent, msg)
}

// suggestSub names the subcommands of parent closest to a word: one slip in
// a short word, two in a longer one, or the start of a name.
func suggestSub(parent *cobra.Command, word string) []string {
	word = strings.ToLower(word)
	var out []string
	for _, c := range parent.Commands() {
		if !c.IsAvailableCommand() || c.Name() == "help" {
			continue
		}
		names := append([]string{c.Name()}, c.Aliases...)
		for _, n := range names {
			if near(word, n) {
				out = append(out, c.CommandPath())
				break
			}
		}
	}
	return out
}

// near says whether a mistyped word is near a name.
func near(word, name string) bool {
	slips := 2
	if len([]rune(word)) <= 4 {
		slips = 1
	}
	return editDistance(word, name) <= slips || (len(word) >= 2 && strings.HasPrefix(name, word)) ||
		(len(word) >= 3 && strings.Contains(name, word))
}

// flagError translates pflag's errors and points at the flag meant.
func flagError(c *cobra.Command, err error) error {
	var notExist *pflag.NotExistError
	var needs *pflag.ValueRequiredError
	var invalid *pflag.InvalidValueError
	switch {
	case errors.As(err, &notExist):
		name := notExist.GetSpecifiedName()
		msg := ""
		if name != "" {
			msg = i18n.T("err.flag_unknown", "--"+name)
			if s := closestFlag(c, name); s != "" {
				msg += " " + i18n.T("err.did_you_mean", "--"+s)
			} else if noAIFlag(name) && !hasFlag(c, name) {
				msg += " " + i18n.T("err.flag_not_here", "--"+name, c.CommandPath())
			}
		} else {
			msg = i18n.T("err.flag_unknown", "-"+notExist.GetSpecifiedShortnames())
		}
		return usageErr(c, msg)
	case errors.As(err, &needs):
		name := "--" + needs.GetSpecifiedName()
		if needs.GetSpecifiedName() == "" {
			name = "-" + needs.GetSpecifiedShortnames()
		}
		return usageErr(c, i18n.T("err.flag_needs_value", name))
	case errors.As(err, &invalid):
		return usageErr(c, i18n.T("err.flag_bad_value", invalid.GetValue(), "--"+invalid.GetFlag().Name))
	}
	return usageErr(c, err.Error())
}

func hasFlag(c *cobra.Command, name string) bool { return c.Flags().Lookup(name) != nil }

// noAIFlag reports a flag of the runs without the AI, which some commands
// (fix, an agent-only task) do not take.
func noAIFlag(name string) bool {
	return slices.Contains([]string{"no-ai", "json", "format", "quiet", "verbose", "fail-on-warning", "fail-on-outdated", "fail-on-critical"}, name)
}

// closestFlag is the flag of c nearest a mistyped name: two slips at
// most, or the start of its name.
func closestFlag(c *cobra.Command, name string) string {
	best, bestD := "", 3
	c.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == name {
			return
		}
		d := editDistance(name, f.Name)
		if len(name) >= 3 && strings.HasPrefix(f.Name, name) {
			d = min(d, 1)
		}
		if d < bestD {
			best, bestD = f.Name, d
		}
	})
	return best
}

// The errors of cobra's argument checks, read back to be said again.
var (
	argsAtMost  = regexp.MustCompile(`accepts at most (\d+) arg\(s\), received (\d+)`)
	argsExact   = regexp.MustCompile(`accepts (\d+) arg\(s\), received (\d+)`)
	argsAtLeast = regexp.MustCompile(`requires at least (\d+) arg\(s\), only received (\d+)`)
	argsBetween = regexp.MustCompile(`accepts between (\d+) and (\d+) arg\(s\), received (\d+)`)
	argsNone    = regexp.MustCompile(`unknown command "(.*)" for "(.*)"`)
	argsInvalid = regexp.MustCompile(`invalid argument "(.*)" for "(.*)"`)
)

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// translatedArgs wraps an argument check so its error is in the user's
// language, ends with the help to read and exits with 64.
func translatedArgs(check cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		err := check(cmd, args)
		if err == nil {
			return nil
		}
		var u usageError
		if errors.As(err, &u) {
			return err
		}
		text := err.Error()
		msg := text
		switch {
		case argsNone.MatchString(text):
			m := argsNone.FindStringSubmatch(text)
			if cmd.HasAvailableSubCommands() {
				return unknownWord(cmd, m[1])
			}
			msg = i18n.T("err.args_none", cmd.CommandPath(), m[1])
		case argsAtMost.MatchString(text):
			m := argsAtMost.FindStringSubmatch(text)
			msg = i18n.N("err.args_at_most", atoi(m[1]), cmd.CommandPath(), atoi(m[1]), atoi(m[2]))
		case argsBetween.MatchString(text):
			m := argsBetween.FindStringSubmatch(text)
			msg = i18n.T("err.args_between", cmd.CommandPath(), atoi(m[1]), atoi(m[2]), atoi(m[3]))
		case argsAtLeast.MatchString(text):
			m := argsAtLeast.FindStringSubmatch(text)
			msg = i18n.N("err.args_at_least", atoi(m[1]), cmd.CommandPath(), atoi(m[1]), atoi(m[2]))
		case argsExact.MatchString(text):
			m := argsExact.FindStringSubmatch(text)
			msg = i18n.N("err.args_exact", atoi(m[1]), cmd.CommandPath(), atoi(m[1]), atoi(m[2]))
		case argsInvalid.MatchString(text):
			m := argsInvalid.FindStringSubmatch(text)
			msg = i18n.T("err.args_invalid", m[1], cmd.CommandPath())
		}
		return usageErr(cmd, msg)
	}
}
