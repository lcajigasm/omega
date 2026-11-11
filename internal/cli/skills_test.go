package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/skills"
)

// skillsHome gives the test an empty home with Claude Code and Codex
// installed, and a folder with one skill to install from.
func skillsHome(t *testing.T) (home, source string) {
	t.Helper()
	isolate(t, nil)
	oldHome, oldLook := skills.Home, skills.LookPath
	home = t.TempDir()
	if real, err := filepath.EvalSymlinks(home); err == nil {
		home = real
	}
	skills.Home = home
	skills.LookPath = func(string) (string, error) { return "", errors.New("not installed") }
	t.Cleanup(func() { skills.Home, skills.LookPath = oldHome, oldLook })
	for _, dir := range []string{".claude", ".codex", "src/pdf"} {
		if err := os.MkdirAll(filepath.Join(home, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	source = filepath.Join(home, "src", "pdf")
	md := "---\nname: pdf\ndescription: Reads PDFs\n---\n"
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	return home, source
}

// omegaSkills runs `omega skills …` and returns what it printed.
func omegaSkills(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var f runFlags
	root := newRoot(&f)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"skills"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestSkillsCommand(t *testing.T) {
	home, source := skillsHome(t)

	out, err := omegaSkills(t)
	if err != nil || !strings.Contains(out, "None installed") || !strings.Contains(out, "Gemini CLI           not installed") {
		t.Fatalf("empty list: %v\n%s", err, out)
	}
	out, err = omegaSkills(t, "install", source)
	if err != nil || !strings.Contains(out, "✓ pdf") || !strings.Contains(out, "~/.agents/skills/pdf") || !strings.Contains(out, "~/.claude/skills/pdf") {
		t.Fatalf("install: %v\n%s", err, out)
	}
	out, err = omegaSkills(t, "list")
	if err != nil || !strings.Contains(out, "Reads PDFs") || !strings.Contains(out, "from: "+source) || !strings.Contains(out, "Claude Code          1 skill ") {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if out, err = omegaSkills(t, "--json"); err != nil || !strings.Contains(out, `"name": "pdf"`) {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if out, err = omegaSkills(t, "audit"); err != nil || !strings.Contains(out, "nothing to report") {
		t.Fatalf("audit: %v\n%s", err, out)
	}
	if out, err = omegaSkills(t, "sync"); err != nil || !strings.Contains(out, "already sees every skill") {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if out, err = omegaSkills(t, "update", "--check"); err != nil || !strings.Contains(out, "pdf: up to date") {
		t.Fatalf("update: %v\n%s", err, out)
	}

	// A skill only Claude has: the audit says who misses it, sync shares it.
	by := filepath.Join(home, ".claude", "skills", "by-hand")
	os.MkdirAll(by, 0o755)
	os.WriteFile(filepath.Join(by, "SKILL.md"), []byte("---\nname: by-hand\n---\n"), 0o644)
	out, _ = omegaSkills(t, "audit")
	if !strings.Contains(out, "no description") || !strings.Contains(out, "not visible to: Codex CLI") || !strings.Contains(out, "1 problem, 1 note") {
		t.Fatalf("audit: %s", out)
	}
	if out, _ = omegaSkills(t, "sync", "--dry-run"); !strings.Contains(out, "would link ~/.agents/skills/by-hand") {
		t.Fatalf("dry run: %s", out)
	}
	if _, err := os.Lstat(filepath.Join(home, ".agents", "skills", "by-hand")); err == nil {
		t.Fatal("the dry run made a change")
	}
	if out, err = omegaSkills(t, "sync", "by-hand"); err != nil || !strings.Contains(out, "✓ ~/.agents/skills/by-hand") {
		t.Fatalf("sync: %v\n%s", err, out)
	}

	// Removing asks; with --yes it does not.
	stdin.Reset(strings.NewReader("n\n"))
	if out, err = omegaSkills(t, "remove", "pdf"); err != nil || !strings.Contains(out, "Nothing was removed") {
		t.Fatalf("declined removal: %v\n%s", err, out)
	}
	if out, err = omegaSkills(t, "remove", "pdf", "--yes"); err != nil || !strings.Contains(out, "✓ pdf removed") {
		t.Fatalf("remove: %v\n%s", err, out)
	}
	if out, _ = omegaSkills(t, "list"); strings.Contains(out, "Reads PDFs") {
		t.Fatalf("pdf is still listed:\n%s", out)
	}
	if _, err = omegaSkills(t, "remove", "pdf", "--yes"); err == nil {
		t.Error("removing a skill that is not installed gave no error")
	}
	if _, err = omegaSkills(t, "install", "--force"); err == nil {
		t.Error("install without a source gave no error")
	}
}

// What the app shows and does is what the command does.
func TestSkillsForTheApp(t *testing.T) {
	t.Cleanup(func() { i18n.Init("en") })
	home, source := skillsHome(t)
	out, err := SkillAction("install", "", source)
	if err != nil || !strings.Contains(out, "1 skill installed") {
		t.Fatalf("install: %v\n%s", err, out)
	}
	broken := filepath.Join(home, ".claude", "skills", "old")
	linkErr := os.Symlink(filepath.Join(home, "gone"), broken)

	i18n.Init("es")
	v, err := SkillList()
	if err != nil || len(v.Agents) != len(skills.Agents) {
		t.Fatalf("list: %+v, %v", v, err)
	}
	pdf := v.Skills[len(v.Skills)-1]
	if pdf.Name != "pdf" || pdf.Source != source || len(pdf.Places) != 2 || strings.Join(pdf.Visible, ",") != "Claude Code,Codex CLI" ||
		pdf.Places[0].Path != "~/.agents/skills/pdf" || pdf.Problems != 0 {
		t.Errorf("pdf: %+v", pdf)
	}
	if linkErr == nil {
		old := v.Skills[0]
		if old.Name != "old" || !old.Places[0].Broken || old.Problems != 1 || !strings.HasPrefix(old.Notes[0].Text, "El enlace apunta") {
			t.Errorf("old: %+v", old)
		}
	}

	if out, err = SkillAction("update", "pdf", ""); err != nil || !strings.Contains(out, "up to date") {
		t.Errorf("update: %v\n%s", err, out)
	}
	if out, err = SkillAction("remove", "pdf", "claude"); err != nil || !strings.Contains(out, "removed") {
		t.Errorf("remove: %v\n%s", err, out)
	}
	if out, err = SkillAction("sync", "", ""); err != nil || !strings.Contains(out, "~/.claude/skills/pdf") {
		t.Errorf("sync: %v\n%s", err, out)
	}
	for _, c := range [][3]string{{"delete", "pdf", ""}, {"remove", "nope", ""}, {"install", "", ""}, {"install", "", "--help"}} {
		if _, err := SkillAction(c[0], c[1], c[2]); err == nil {
			t.Errorf("%v gave no error", c)
		}
	}
}

// Every finding of the audit has a text on the command line and in every
// language of the app.
func TestSkillFindingsHaveTexts(t *testing.T) {
	t.Cleanup(func() { i18n.Init("en") })
	for _, code := range []string{"broken-link", "no-frontmatter", "no-name", "bad-name", "name-mismatch",
		"no-description", "long-description", "differs", "hidden"} {
		if text := (skills.Finding{Code: code}).Text(); text == code {
			t.Errorf("%s has no text on the command line", code)
		}
		for _, lang := range i18n.Supported {
			i18n.Init(lang)
			if key := "gui.skills.f." + code; i18n.T(key) == key {
				t.Errorf("%s has no text in %s", code, lang)
			}
		}
	}
}
