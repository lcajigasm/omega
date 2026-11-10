package cli

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/agent"
)

var (
	// `omega mac undo` in running text, <code>omega dev status</code> on the site.
	docInline = regexp.MustCompile("`(omega(?: [^`]*)?)`|<code>(omega(?: [^<]*)?)</code>")
	docBlock  = regexp.MustCompile("(?s)```[a-z]*\\n(.*?)```|<pre[^>]*><code>(.*?)</code></pre>")
	// A line of a code block that starts with the command.
	docLine = regexp.MustCompile(`(?m)^\s*(omega(?: .*)?)$`)
)

// documentedCommands returns every omega invocation a document shows, as the
// words before the first flag, placeholder or comment.
func documentedCommands(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	add := func(cmd string) {
		var words []string
		for _, w := range strings.Fields(cmd)[1:] {
			// Stop at flags, comments, placeholders, quotes and shell syntax.
			if !regexp.MustCompile(`^[a-z][a-z0-9-]*$`).MatchString(w) {
				break
			}
			words = append(words, w)
		}
		seen[strings.Join(words, " ")] = true
	}
	for _, m := range docInline.FindAllStringSubmatch(string(data), -1) {
		add(m[1] + m[2])
	}
	// Lines of code blocks: fenced in Markdown, <pre><code> in HTML. Prose
	// lines that begin with the word omega are not commands.
	for _, block := range docBlock.FindAllStringSubmatch(string(data), -1) {
		for _, m := range docLine.FindAllStringSubmatch(block[1]+block[2], -1) {
			add(m[1])
		}
	}
	var out []string
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// Every command the README and the website show must exist in this binary.
// The reverse failure is what a user meets first: they copy a command from
// the documentation and omega says it does not know it.
func TestDocumentedCommandsExist(t *testing.T) {
	var f runFlags
	root := newRoot(&f)
	root.InitDefaultCompletionCmd() // cobra adds `omega completion` when it runs
	for _, doc := range []string{"../../README.md", "../../docs/index.html"} {
		cmds := documentedCommands(t, doc)
		if len(cmds) < 10 {
			t.Fatalf("%s: only %d commands found, the extraction is broken", doc, len(cmds))
		}
		for _, c := range cmds {
			if c == "" {
				continue
			}
			words := strings.Fields(c)
			cmd, rest, err := root.Find(words)
			if err != nil {
				t.Errorf("%s shows `omega %s`: %v", doc, c, err)
				continue
			}
			switch {
			case len(rest) == 0:
			case cmd == root:
				// A task, optionally followed by free words the extraction kept.
				if _, ok := agent.TaskPrompts[rest[0]]; !ok {
					t.Errorf("%s shows `omega %s`, but %q is neither a command nor a task", doc, c, rest[0])
				}
			case cmd.HasSubCommands():
				t.Errorf("%s shows `omega %s`, but `omega %s` has no subcommand %q", doc, c, strings.TrimPrefix(cmd.CommandPath(), "omega "), rest[0])
			}
		}
	}
}

func TestUnknownCommandSaysHowToUpdate(t *testing.T) {
	old := version
	version = "v0.13.0"
	t.Cleanup(func() { version = old })
	var f runFlags
	root := newRoot(&f)
	for _, args := range [][]string{{"menuu"}, {"dev2", "status"}, {"update", "now"}} {
		root.SetArgs(args)
		err := root.Execute()
		if err == nil {
			t.Fatalf("%v: no error", args)
		}
		for _, want := range []string{args[0], "v0.13.0", "omega --help", "brew upgrade omega"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%v: %q is missing from %q", args, want, err.Error())
			}
		}
	}
}

func TestDoctorVersion(t *testing.T) {
	oldV, oldL := version, latestRelease
	t.Cleanup(func() { version, latestRelease = oldV, oldL })
	for _, c := range []struct{ mine, latest, want string }{
		{"v0.13.0", "v0.15.0", "v0.15.0 is available"},
		{"v0.15.0", "v0.15.0", "(latest)"},
		{"v0.15.0-3-gabc1234-dirty", "v0.15.0", "(latest)"},
		{"v0.16.0", "v0.15.0", "(latest)"},
		{"v0.9.9", "v0.10.0", "v0.10.0 is available"},
		{"v0.13.0", "", "version: v0.13.0"},
		{"dev", "v0.15.0", "version: dev"},
	} {
		version = c.mine
		latestRelease = func() string { return c.latest }
		var b strings.Builder
		doctorVersion(&b)
		if !strings.Contains(b.String(), c.want) || (c.latest == "" || c.mine == "dev") && strings.Contains(b.String(), "available") {
			t.Errorf("%s vs %q: %q", c.mine, c.latest, b.String())
		}
	}
}
