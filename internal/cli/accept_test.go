package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/ui"
)

func testRoot() *cobra.Command {
	var f runFlags
	return newRoot(&f)
}

func TestInterpretAcceptsOtherWordsAndSentences(t *testing.T) {
	root := testRoot()
	for words, want := range map[string]string{
		"":           agent.TaskFull,
		"update":     agent.TaskUpdate,
		"UPDATE":     agent.TaskUpdate,
		"upgrade":    agent.TaskUpdate,
		"actualizar": agent.TaskUpdate,
		"upd":        agent.TaskUpdate, // the only task that starts this way
		"limpiar":    agent.TaskClean,
		"cleanup":    agent.TaskClean,
		"analyse":    agent.TaskAnalyze,
		"informe":    agent.TaskAnalyze,
		"repair":     agent.TaskFix,
		"optimizar":  agent.TaskTune,
		"todo":       agent.TaskFull,
	} {
		task, prompt, err := interpret(root, strings.Fields(words))
		if err != nil || task != want || prompt != "" {
			t.Errorf("%q: task %q prompt %q err %v, want task %q", words, task, prompt, err, want)
		}
	}
	// A sentence is a request for the agent, typed or quoted.
	for _, args := range [][]string{{"why", "is", "my", "disk", "full"}, {"limpia las cachés de npm"}} {
		task, prompt, err := interpret(root, args)
		if err != nil || task != "" || prompt != strings.Join(args, " ") {
			t.Errorf("%v: task %q prompt %q err %v", args, task, prompt, err)
		}
	}
	// One or two unknown words are not guessed at: a typo must not start a
	// paid session.
	for _, args := range [][]string{{"updat3"}, {"f"}, {"dev2", "status"}, {"update", "now"}} {
		if _, _, err := interpret(root, args); err == nil {
			t.Errorf("%v: accepted", args)
		}
	}
}

func TestUnknownWordSuggestsTheNearestCommand(t *testing.T) {
	root := testRoot()
	for word, want := range map[string]string{
		"menuu": "omega menu", "cofig": "omega config", "updte": "omega update", "docter": "omega doctor",
		"anlyze": "omega analyze", "model": "omega models", "perm": "",
	} {
		got := strings.Join(suggest(root, word), ", ")
		if want != "" && !strings.Contains(got, want) {
			t.Errorf("%q suggests %q, want %q", word, got, want)
		}
	}
	if s := suggest(root, "doc"); strings.Contains(strings.Join(s, " "), "omega mac") {
		t.Errorf("a short word should not suggest names two slips away: %v", s)
	}
	_, _, err := interpret(root, []string{"menuu"})
	if err == nil || !strings.Contains(err.Error(), "Did you mean omega menu?") || !strings.Contains(err.Error(), `omega -p`) {
		t.Fatalf("message: %v", err)
	}
}

func TestShortcutsExpandToRealCommands(t *testing.T) {
	root := testRoot()
	root.InitDefaultCompletionCmd()
	for word, want := range map[string]string{
		"perms": "mac permissions", "permisos": "mac permissions", "undo": "mac undo", "install": "dev install",
		"historial": "logs", "ajustes": "config show", "status": "doctor", "claves": "auth status", "inicio": "menu",
	} {
		if got := strings.Join(expandShortcut(root, []string{word}), " "); got != want {
			t.Errorf("%q expands to %q, want %q", word, got, want)
		}
	}
	// Arguments after the shortcut are kept.
	if got := strings.Join(expandShortcut(root, []string{"install", "fzf", "mise"}), " "); got != "dev install fzf mise" {
		t.Errorf("got %q", got)
	}
	// Real commands, tasks and flags are never rewritten.
	for _, args := range [][]string{{"doctor"}, {"update"}, {"--version"}, {"-p", "undo the last change"}, {"mac", "undo"}, nil} {
		if got := expandShortcut(root, args); strings.Join(got, " ") != strings.Join(args, " ") {
			t.Errorf("%v rewritten to %v", args, got)
		}
	}
	// Every shortcut leads somewhere that exists.
	for word, to := range shortcuts {
		if strings.HasPrefix(to[0], "-") {
			continue
		}
		c, rest, err := root.Find(to)
		if err != nil || c == root || len(rest) != 0 {
			t.Errorf("shortcut %q -> %v does not resolve (%v, rest %v)", word, to, err, rest)
		}
		for _, c := range root.Commands() {
			if c.Name() == word || c.HasAlias(word) {
				t.Errorf("shortcut %q hides the real command %q", word, c.Name())
			}
		}
	}
	// A shortcut wins over the start of another command's name.
	if got := strings.Join(expandShortcut(root, []string{"dock"}), " "); got != "mac dock" {
		t.Errorf("dock expands to %q", got)
	}
	for word, task := range taskWords {
		if _, ok := agent.TaskPrompts[task]; !ok {
			t.Errorf("task word %q -> unknown task %q", word, task)
		}
		for _, c := range root.Commands() {
			if c.Name() == word || c.HasAlias(word) {
				t.Errorf("task word %q is also the command %q", word, c.Name())
			}
		}
	}
}

// menuLeaves lists every row of the menu that runs something.
func menuLeaves(items []ui.MenuItem) []ui.MenuItem {
	var out []ui.MenuItem
	for _, it := range items {
		if len(it.Children) == 0 {
			out = append(out, it)
		}
		out = append(out, menuLeaves(it.Children)...)
		out = append(out, menuLeaves(it.Options)...)
	}
	return out
}

func TestMenuOffersEveryCommand(t *testing.T) {
	old := goos
	goos = "darwin"
	t.Cleanup(func() { goos = old })
	root := testRoot()
	leaves := menuLeaves(menuTree(root))
	offered := map[string]bool{}
	for _, l := range leaves {
		offered[strings.Join(l.Args, " ")] = true
		// A row that asks for a value covers the command it completes.
		for i := range l.Args {
			offered[strings.Join(l.Args[:i+1], " ")+" …"] = true
		}
	}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if sub.Hidden || sub.Name() == "help" || sub.Name() == "menu" {
				continue
			}
			path := strings.TrimPrefix(sub.CommandPath(), "omega ")
			if sub.Runnable() && !offered[path] && !offered[path+" …"] {
				reached := false
				for o := range offered {
					if strings.HasPrefix(o, path+" ") {
						reached = true
					}
				}
				if !reached {
					t.Errorf("the menu has no row for `omega %s`", path)
				}
			}
			walk(sub)
		}
	}
	walk(root)

	// Every row runs a command line this binary accepts.
	for _, l := range leaves {
		args := l.Args
		if len(args) == 0 || strings.HasPrefix(args[0], "-") {
			continue
		}
		c, rest, err := root.Find(args)
		if err != nil {
			t.Errorf("%q: %v", l.Label, err)
			continue
		}
		if c == root {
			if _, ok := taskFor(rest[0]); !ok {
				t.Errorf("%q is neither a command nor a task", l.Label)
			}
		}
	}
	for _, want := range []string{"update --dry-run", "config set language es", "config set approval never", "auth set anthropic", "mac permissions", "dev activate", "logs view", "notes clear", "models --refresh"} {
		if !offered[want] {
			t.Errorf("the menu lacks `omega %s`", want)
		}
	}

	// Off macOS the Mac review is left out.
	goos = "linux"
	for _, l := range menuLeaves(menuTree(testRoot())) {
		if len(l.Args) > 0 && l.Args[0] == "mac" {
			t.Fatalf("`omega %s` is offered on Linux", strings.Join(l.Args, " "))
		}
	}
}
