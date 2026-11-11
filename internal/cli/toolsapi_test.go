package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/i18n"
)

// Every tool the desktop app lists is a command omega has, with a title and
// a description in every language.
func TestToolListMatchesCommands(t *testing.T) {
	t.Cleanup(func() { i18n.Init("en") })
	var f runFlags
	root := newRoot(&f)
	seen := map[string]bool{}
	for _, tool := range guiTools {
		if seen[tool.id] {
			t.Errorf("%s is listed twice", tool.id)
		}
		seen[tool.id] = true
		c, rest, err := root.Find(tool.args)
		if err != nil || c == root || len(rest) != 0 {
			t.Errorf("%s: omega %s is not a command (%v, left %v)", tool.id, strings.Join(tool.args, " "), err, rest)
		}
	}
	for _, lang := range i18n.Supported {
		i18n.Init(lang)
		for _, in := range ToolList() {
			for _, text := range []string{in.Group, in.Title, in.Detail} {
				if text == "" || strings.HasPrefix(text, "gui.") {
					t.Errorf("%s (%s): a text is missing: %q", in.ID, lang, text)
				}
			}
			if (in.Arg != "") && strings.HasPrefix(in.Arg, "gui.") {
				t.Errorf("%s (%s): the argument has no label", in.ID, lang)
			}
		}
	}
}

func TestRunTool(t *testing.T) {
	isolate(t, nil)
	out, err := RunTool("schedule-status", "")
	if err != nil || !strings.Contains(out, "Background Maintenance Status") {
		t.Errorf("schedule status: %q, %v", out, err)
	}
	// The app can run what is listed and nothing else: no other command, and
	// no flag slipped in as an argument.
	if _, err := RunTool("uninstall", "Safari"); err == nil {
		t.Error("a command that is not a tool ran")
	}
	if _, err := RunTool("disk-large", "--help"); err == nil {
		t.Error("a flag was accepted as an argument")
	}
	// What a tool finds comes back as text, not on the app's own stdout.
	dir := t.TempDir()
	out, err = RunTool("disk-large", dir)
	if err != nil || !strings.Contains(out, filepath.Base(dir)) {
		t.Errorf("disk large: %q, %v", out, err)
	}
}
