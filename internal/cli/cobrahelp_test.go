package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/i18n"
)

func usageCode(err error) int {
	var code interface{ ExitCode() int }
	if errors.As(err, &code) {
		return code.ExitCode()
	}
	return -1
}

// The help is in the user's language, cobra's words included.
func TestHelpIsTranslated(t *testing.T) {
	isolate(t, nil)
	t.Setenv("OMEGA_LANG", "es")
	i18n.Init("es")
	t.Cleanup(func() { i18n.Init("en") })
	root := testRoot()
	var out strings.Builder
	root.SetOut(&out)
	root.SetArgs([]string{"logs", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, want := range []string{"Uso:", "Opciones:", "ayuda de omega logs", "Ver también: omega history"} {
		if !strings.Contains(help, want) {
			t.Errorf("logs --help lacks %q:\n%s", want, help)
		}
	}
	for _, english := range []string{"Usage:", "Flags:", "help for", "Global Flags:"} {
		if strings.Contains(help, english) {
			t.Errorf("logs --help still says %q:\n%s", english, help)
		}
	}
}

// The flags of a task are in two groups, and 0 is said to mean no limit.
func TestTaskHelpGroupsFlags(t *testing.T) {
	out, err := execRoot(t, "health", "--help")
	if err != nil {
		t.Fatal(err)
	}
	agent, noAI := strings.Index(out, "Agent options:"), strings.Index(out, "Options without AI")
	if agent < 0 || noAI < 0 {
		t.Fatalf("no groups:\n%s", out)
	}
	if i := strings.Index(out, "--fail-on-warning"); i < noAI {
		t.Errorf("--fail-on-warning is not with the options without AI:\n%s", out)
	}
	if i := strings.Index(out, "--max-cost"); i < agent || i > noAI {
		t.Errorf("--max-cost is not with the agent's options:\n%s", out)
	}
	if !strings.Contains(out, "0: no limit") {
		t.Errorf("--max-cost does not say what 0 means:\n%s", out)
	}
	// health has a help of its own, not the root's.
	if strings.Contains(out, "Tasks:") {
		t.Errorf("omega health --help shows the root help:\n%s", out)
	}
}

// A command line omega cannot use exits with 64 and says which help to read.
func TestUsageErrorsExit64(t *testing.T) {
	isolate(t, nil)
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"logs", "--bogus"}, "unknown option --bogus"},
		{[]string{"logs", "--lst"}, "--last"},
		{[]string{"logs", "-n"}, "needs a value"},
		{[]string{"logs", "-n", "x"}, "not a valid value"},
		{[]string{"doctor", "extra"}, "takes no arguments"},
		{[]string{"logs", "view", "1", "2"}, "at most 1 argument"},
		{[]string{"pending", "snooze", "x"}, "takes 2 arguments"},
		{[]string{"config", "sett"}, "Did you mean omega config set?"},
		{[]string{"help", "bogus"}, "unknown command"},
		{[]string{"help", "config", "sett"}, "omega config set"},
		{[]string{"dsk"}, "Did you mean omega disk?"},
		{[]string{"--format", "yaml", "clean"}, "yaml"},
	} {
		_, err := execRoot(t, c.args...)
		if err == nil || !strings.Contains(err.Error(), c.want) || usageCode(err) != exitUsage {
			t.Errorf("omega %v: %v (exit %d), want %q and 64", c.args, err, usageCode(err), c.want)
			continue
		}
		if !strings.Contains(err.Error(), "--help") {
			t.Errorf("omega %v does not say which help to read: %v", c.args, err)
		}
	}
	// A close name is offered without "update omega".
	if _, err := execRoot(t, "dsk"); err != nil && strings.Contains(err.Error(), "brew upgrade") {
		t.Errorf("a slip is answered with an update: %v", err)
	}
}

// The tasks are commands with their own help, and still answer as before.
func TestTasksAreCommands(t *testing.T) {
	root := testRoot()
	for _, task := range []string{"update", "clean", "analyze", "health", "tune"} {
		c, rest, err := root.Find([]string{task})
		if err != nil || c.Name() != task || len(rest) != 0 || c.Long == "" {
			t.Errorf("%s is not a command of its own: %v %v", task, c.Name(), err)
		}
	}
	if len(root.ValidArgs) != 0 {
		t.Errorf("the root still completes the tasks as words: %v", root.ValidArgs)
	}
	// The words after a task are read as before: a sentence is a request.
	task, prompt, err := interpret(root, []string{"update", "brew", "and", "npm"})
	if err != nil || task != "" || prompt != "update brew and npm" {
		t.Errorf("task %q prompt %q err %v", task, prompt, err)
	}
}

// fix takes only the agent's flags.
func TestFixHasOnlyAgentFlags(t *testing.T) {
	c, _, _ := testRoot().Find([]string{"fix"})
	for _, name := range []string{"no-ai", "json", "format", "quiet", "verbose", "fail-on-warning", "fail-on-outdated", "fail-on-critical"} {
		if c.Flags().Lookup(name) != nil {
			t.Errorf("fix has --%s", name)
		}
	}
	for _, name := range []string{"provider", "model", "yes", "log", "max-cost"} {
		if c.Flags().Lookup(name) == nil {
			t.Errorf("fix lacks --%s", name)
		}
	}
	if !strings.Contains(c.Long, "omega fix -") || !strings.Contains(c.Long, "--log") {
		t.Errorf("fix's help does not say how to give it an error:\n%s", c.Long)
	}
}

// Every Short fits the column of `omega --help`, in every language.
func TestShortsAreShort(t *testing.T) {
	t.Cleanup(func() { i18n.Init("en") })
	for _, lang := range i18n.Supported {
		restore := i18n.Use(lang)
		root := testRoot()
		var walk func(c *cobra.Command)
		walk = func(c *cobra.Command) {
			for _, sub := range c.Commands() {
				if !sub.Hidden {
					if w := lipgloss.Width(sub.Short); w > 60 {
						t.Errorf("%s/%s: Short is %d columns: %q", lang, sub.CommandPath(), w, sub.Short)
					}
				}
				walk(sub)
			}
		}
		walk(root)
		restore()
	}
}

// The values of the flags that have a known set are completed.
func TestFlagCompletions(t *testing.T) {
	isolate(t, nil)
	out, err := execRoot(t, cobra.ShellCompRequestCmd, "clean", "--effort", "")
	if err != nil || !strings.Contains(out, "xhigh") {
		t.Errorf("--effort: %v\n%s", err, out)
	}
	out, _ = execRoot(t, cobra.ShellCompRequestCmd, "--provider", "")
	if !strings.Contains(out, "anthropic") || !strings.Contains(out, "openrouter") {
		t.Errorf("--provider:\n%s", out)
	}
	out, _ = execRoot(t, cobra.ShellCompRequestCmd, "steps", "show", "")
	if !strings.Contains(out, "disk") {
		t.Errorf("steps show ids:\n%s", out)
	}
	out, _ = execRoot(t, cobra.ShellCompRequestCmd, "")
	if strings.Count(out, "\nupdate\t") > 1 || strings.Count(out, "\ndev\t") > 1 {
		t.Errorf("a command is offered twice:\n%s", out)
	}
	if c, _, _ := testRoot().Find([]string{"completion"}); c.Hidden {
		t.Error("completion is hidden from the help")
	}
}

// The man page is written in English whatever the language, and only
// where --output says.
func TestManPage(t *testing.T) {
	t.Cleanup(func() { i18n.Init("en") })
	i18n.Init("es")
	page := englishManPage()
	if !strings.Contains(page, "EXIT STATUS") || !strings.Contains(page, "64") {
		t.Error("the man page does not document the exit status")
	}
	if strings.Contains(page, "Opciones") || i18n.Lang() != "es" {
		t.Error("the man page was not written in English, or the language was not restored")
	}
	dir := t.TempDir()
	out, err := execRoot(t, "man", "--output", dir)
	if err != nil || !strings.Contains(out, dir) {
		t.Errorf("man --output: %v %s", err, out)
	}
	// Not at a terminal: the page goes to the output.
	out, err = execRoot(t, "man")
	if err != nil || !strings.HasPrefix(out, ".TH OMEGA 1") {
		t.Errorf("man: %v %.80s", err, out)
	}
}
