package skills

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// machine gives the test an empty home where the named agents are
// installed, and returns the home.
func machine(t *testing.T, agents ...string) string {
	t.Helper()
	oldHome, oldLook := Home, LookPath
	home := t.TempDir()
	// On a Mac the temporary folder is itself behind a link.
	if real, err := filepath.EvalSymlinks(home); err == nil {
		home = real
	}
	Home = home
	LookPath = func(bin string) (string, error) {
		if bin == "git" {
			return oldLook(bin)
		}
		return "", errors.New("not installed")
	}
	t.Cleanup(func() { Home, LookPath = oldHome, oldLook })
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	for _, id := range agents {
		a, ok := AgentByID(id)
		if !ok {
			t.Fatalf("no agent %q", id)
		}
		if err := os.MkdirAll(Expand(a.Home), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

// write creates a skill folder with a SKILL.md and returns the folder.
func write(t *testing.T, dir, name, description string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Join(path, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# " + name + "\n"
	for file, text := range map[string]string{"SKILL.md": md, "references/notes.md": "notes\n"} {
		if err := os.WriteFile(filepath.Join(path, file), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func scan(t *testing.T) *Inventory {
	t.Helper()
	inv, err := Scan()
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

func skill(t *testing.T, name string) *Skill {
	t.Helper()
	s, ok := scan(t).Find(name)
	if !ok {
		t.Fatalf("%s is not installed", name)
	}
	return s
}

func codes(s *Skill) string {
	var out []string
	for _, f := range s.Findings {
		out = append(out, f.Code)
	}
	return strings.Join(out, ",")
}

func TestFrontmatter(t *testing.T) {
	for _, c := range []struct{ text, name, desc string }{
		{"---\nname: pdf\ndescription: Reads PDFs\n---\nbody", "pdf", "Reads PDFs"},
		{"---\r\nname: \"pdf\"\r\ndescription: 'Reads: PDFs'\r\n---\r\n", "pdf", "Reads: PDFs"},
		{"---\nname: pdf\ndescription: >\n  Reads PDFs\n  and fills forms\nmetadata:\n  version: 2\n---\n", "pdf", "Reads PDFs and fills forms"},
		{"---\nname: pdf\ndescription:\n  Reads PDFs\n---\n", "pdf", "Reads PDFs"},
	} {
		name, desc, ok := frontmatter(c.text)
		if !ok || name != c.name || desc != c.desc {
			t.Errorf("%q: %q, %q, %v", c.text, name, desc, ok)
		}
	}
	for _, text := range []string{"# pdf\n", "---\nname: pdf\n", ""} {
		if _, _, ok := frontmatter(text); ok {
			t.Errorf("%q was taken for a frontmatter block", text)
		}
	}
}

// Who sees a skill follows from where it is: the shared folder reaches
// everyone but Claude Code, Claude's folder reaches the agents that read it.
func TestScanSaysWhoSeesASkill(t *testing.T) {
	home := machine(t, "claude", "codex", "opencode")
	write(t, filepath.Join(home, ".agents", "skills"), "shared-one", "For everyone")
	write(t, filepath.Join(home, ".claude", "skills"), "claude-one", "For Claude")
	// Not skills: a folder without SKILL.md, a file, a hidden folder.
	os.MkdirAll(filepath.Join(home, ".claude", "skills", "synced", "bucket"), 0o755)
	os.WriteFile(filepath.Join(home, ".claude", "skills", "README.md"), []byte("x"), 0o644)
	write(t, filepath.Join(home, ".claude", "skills"), ".hidden", "No")

	inv := scan(t)
	if len(inv.Skills) != 2 {
		t.Fatalf("skills: %+v", inv.Skills)
	}
	s, _ := inv.Find("shared-one")
	if strings.Join(s.Visible, ",") != "codex,opencode" || strings.Join(s.Hidden, ",") != "claude" || codes(s) != "hidden" {
		t.Errorf("shared-one: visible %v, hidden %v, findings %s", s.Visible, s.Hidden, codes(s))
	}
	s, _ = inv.Find("claude-one")
	if strings.Join(s.Visible, ",") != "claude,opencode" || strings.Join(s.Hidden, ",") != "codex" || s.Description != "For Claude" {
		t.Errorf("claude-one: visible %v, hidden %v, %q", s.Visible, s.Hidden, s.Description)
	}
	for _, a := range inv.Agents {
		want := map[string]int{"claude": 1, "codex": 1, "opencode": 2}[a.ID]
		if a.Skills != want || a.Installed != (want > 0) {
			t.Errorf("%s: installed %v, sees %d, want %d", a.ID, a.Installed, a.Skills, want)
		}
	}
}

func TestAuditFindings(t *testing.T) {
	home := machine(t, "claude")
	dir := filepath.Join(home, ".claude", "skills")
	write(t, dir, "good", "Fine")
	for name, md := range map[string]string{
		"no-block":  "# Just a title\n",
		"no-desc":   "---\nname: no-desc\n---\n",
		"renamed":   "---\nname: other-name\ndescription: x\n---\n",
		"bad-name":  "---\nname: Bad Name\ndescription: x\n---\n",
		"long-desc": "---\nname: long-desc\ndescription: " + strings.Repeat("x", 1025) + "\n---\n",
	} {
		os.MkdirAll(filepath.Join(dir, name), 0o755)
		os.WriteFile(filepath.Join(dir, name, "SKILL.md"), []byte(md), 0o644)
	}
	inv := scan(t)
	for name, want := range map[string]string{"good": "", "no-block": "no-frontmatter", "no-desc": "no-description",
		"renamed": "name-mismatch", "bad-name": "bad-name", "long-desc": "long-description"} {
		s, ok := inv.Find(name)
		if !ok || codes(s) != want {
			t.Errorf("%s: %q, want %q", name, codes(s), want)
		}
	}
	if s, _ := inv.Find("renamed"); s.Problems() != 0 {
		t.Error("a name that differs from the folder is a note, not a problem")
	}
}

func TestAuditCopiesThatDiffer(t *testing.T) {
	home := machine(t, "claude", "codex")
	write(t, filepath.Join(home, ".agents", "skills"), "pdf", "One")
	write(t, filepath.Join(home, ".claude", "skills"), "pdf", "One")
	if s := skill(t, "pdf"); codes(s) != "" {
		t.Fatalf("identical copies: %s", codes(s))
	}
	os.WriteFile(filepath.Join(home, ".claude", "skills", "pdf", "references", "notes.md"), []byte("edited\n"), 0o644)
	s := skill(t, "pdf")
	if codes(s) != "differs" || s.Problems() != 1 {
		t.Fatalf("edited copy: %s", codes(s))
	}
	// Sync does not choose between them.
	actions, skipped, err := PlanSync(scan(t), nil)
	if err != nil || len(actions) != 0 || len(skipped) != 0 {
		t.Errorf("both agents see it, nothing to do: %v, %v, %v", actions, skipped, err)
	}
}

func TestSyncSharesWithEveryAgent(t *testing.T) {
	home := machine(t, "claude", "codex", "gemini", "copilot")
	write(t, filepath.Join(home, ".claude", "skills"), "from-claude", "x")
	write(t, filepath.Join(home, ".agents", "skills"), "from-shared", "x")

	actions, skipped, err := PlanSync(scan(t), nil)
	if err != nil || len(skipped) != 0 {
		t.Fatal(err, skipped)
	}
	// One link each: the shared folder covers Codex, Gemini and Copilot.
	got := map[string]string{}
	for _, a := range actions {
		got[a.Skill] += a.Location
	}
	if len(actions) != 2 || got["from-claude"] != Shared || got["from-shared"] != "claude" {
		t.Fatalf("plan: %+v", actions)
	}
	for _, a := range actions {
		if _, err := Apply(a, false); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range scan(t).Skills {
		if len(s.Hidden) != 0 || len(s.Visible) != 4 || codes(&s) != "" {
			t.Errorf("%s after sync: hidden %v, findings %s", s.Name, s.Hidden, codes(&s))
		}
	}
	if actions, _, _ := PlanSync(scan(t), nil); len(actions) != 0 {
		t.Errorf("a second sync still has work: %+v", actions)
	}
	if _, _, err := PlanSync(scan(t), []string{"nope"}); err == nil {
		t.Error("an unknown skill was accepted")
	}
	// Nothing in the way is overwritten.
	if _, err := Apply(Action{Skill: "x", Path: filepath.Join(home, ".claude", "skills", "from-claude"), Source: home}, false); err == nil {
		t.Error("an existing skill was overwritten")
	}
}

func TestBrokenLinkIsFoundAndRepaired(t *testing.T) {
	home := machine(t, "claude", "codex")
	src := write(t, filepath.Join(home, ".agents", "skills"), "pdf", "x")
	gone := filepath.Join(home, "gone")
	link := filepath.Join(home, ".claude", "skills", "pdf")
	os.MkdirAll(filepath.Dir(link), 0o755)
	if err := os.Symlink(gone, link); err != nil {
		t.Skip("this system does not allow links:", err)
	}
	s := skill(t, "pdf")
	if !strings.Contains(codes(s), "broken-link") || strings.Join(s.Hidden, ",") != "claude" {
		t.Fatalf("findings %s, hidden %v", codes(s), s.Hidden)
	}
	actions, _, _ := PlanSync(scan(t), nil)
	if len(actions) != 1 {
		t.Fatalf("plan: %+v", actions)
	}
	if linked, err := Apply(actions[0], false); err != nil || !linked {
		t.Fatal(linked, err)
	}
	if s := skill(t, "pdf"); codes(s) != "" || s.Installs[1].Real != src {
		t.Errorf("after repair: %s, %+v", codes(s), s.Installs)
	}
}

func TestInstallFromFolderUpdateAndRemove(t *testing.T) {
	home := machine(t, "claude", "codex")
	repo := filepath.Join(home, "src")
	write(t, filepath.Join(repo, "skills"), "pdf", "Reads PDFs")
	write(t, filepath.Join(repo, "skills"), "xlsx", "Reads sheets")
	ctx := context.Background()

	if _, err := InstallFrom(ctx, repo, InstallOptions{Only: []string{"nope"}}); err == nil || !strings.Contains(err.Error(), "pdf, xlsx") {
		t.Fatalf("a skill the source lacks: %v", err)
	}
	done, err := InstallFrom(ctx, repo, InstallOptions{Only: []string{"pdf"}})
	if err != nil || len(done) != 1 || len(done[0].Paths) != 2 {
		t.Fatalf("install: %+v, %v", done, err)
	}
	s := skill(t, "pdf")
	if len(s.Hidden) != 0 || codes(s) != "" || s.Origin == nil || s.Origin.Dir != filepath.Join(repo, "skills", "pdf") {
		t.Fatalf("installed: hidden %v, findings %s, origin %+v", s.Hidden, codes(s), s.Origin)
	}
	if s.Installs[0].Location != Shared || s.Installs[0].Link != "" {
		t.Errorf("the files belong in the shared folder: %+v", s.Installs)
	}
	if _, ok := scan(t).Find("xlsx"); ok {
		t.Error("a skill that was not asked for was installed")
	}
	if _, err := InstallFrom(ctx, repo, InstallOptions{Only: []string{"pdf"}}); err == nil {
		t.Error("installing twice replaced the skill without --force")
	}

	// Nothing new at the source.
	res, err := Update(ctx, nil, false, false)
	if err != nil || len(res) != 1 || res[0].State != Current {
		t.Fatalf("update: %+v, %v", res, err)
	}
	// The source changes: check sees it, update brings it.
	os.WriteFile(filepath.Join(repo, "skills", "pdf", "references", "notes.md"), []byte("v2\n"), 0o644)
	if res, _ := Update(ctx, nil, true, false); res[0].State != Available {
		t.Fatalf("check: %+v", res)
	}
	if res, _ := Update(ctx, []string{"pdf"}, false, false); res[0].State != Updated {
		t.Fatalf("update: %+v", res)
	}
	got, _ := os.ReadFile(filepath.Join(home, ".claude", "skills", "pdf", "references", "notes.md"))
	if string(got) != "v2\n" {
		t.Errorf("Claude's folder holds %q", got)
	}
	// An edit made here is not thrown away.
	os.WriteFile(filepath.Join(home, ".agents", "skills", "pdf", "mine.md"), []byte("mine\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "skills", "pdf", "references", "notes.md"), []byte("v3\n"), 0o644)
	if res, _ := Update(ctx, nil, false, false); res[0].State != Modified {
		t.Fatalf("an edited skill: %+v", res)
	}
	if res, _ := Update(ctx, nil, false, true); res[0].State != Updated {
		t.Fatalf("forced: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "pdf", "mine.md")); err == nil {
		t.Error("the forced update kept a file the source does not have")
	}

	// A skill omega did not install has nowhere to update from.
	write(t, filepath.Join(home, ".claude", "skills"), "by-hand", "x")
	if res, _ := Update(ctx, []string{"by-hand"}, false, false); res[0].State != Untracked {
		t.Errorf("by-hand: %+v", res)
	}

	// Removing takes the folder and the link, and forgets the origin.
	plan, err := PlanRemove(scan(t), "pdf", "")
	if err != nil || len(plan) != 2 {
		t.Fatalf("plan: %+v, %v", plan, err)
	}
	kept, err := Remove("pdf", plan)
	if err != nil || len(kept) == 0 { // one folder, or two where links are copies
		t.Fatalf("remove: %v, %v", kept, err)
	}
	if _, err := os.Stat(filepath.Join(kept[0], "SKILL.md")); err != nil {
		t.Errorf("the folder is not where Remove said: %v", err)
	}
	if _, ok := scan(t).Find("pdf"); ok {
		t.Error("pdf is still installed")
	}
	if _, tracked := loadOrigins()["pdf"]; tracked {
		t.Error("the origin of a removed skill was kept")
	}
	if _, err := PlanRemove(scan(t), "pdf", ""); err == nil {
		t.Error("removing a skill that is not installed gave no error")
	}
}

func TestRemoveFromOneLocation(t *testing.T) {
	home := machine(t, "claude", "codex")
	write(t, filepath.Join(home, ".agents", "skills"), "pdf", "x")
	actions, _, _ := PlanSync(scan(t), nil)
	linked, err := Apply(actions[0], false)
	if err != nil {
		t.Fatal(err)
	}
	// Only Claude's entry goes.
	plan, err := PlanRemove(scan(t), "pdf", "claude")
	if err != nil || len(plan) != 1 || plan[0].Location != "claude" {
		t.Fatalf("plan: %+v, %v", plan, err)
	}
	if _, err := Remove("pdf", plan); err != nil {
		t.Fatal(err)
	}
	if s := skill(t, "pdf"); len(s.Installs) != 1 || s.Installs[0].Location != Shared {
		t.Fatalf("after removing from claude: %+v", s.Installs)
	}
	// Removing the folder takes the links to it along.
	Apply(actions[0], false)
	plan, _ = PlanRemove(scan(t), "pdf", Shared)
	if want := map[bool]int{true: 2, false: 1}[linked]; len(plan) != want {
		t.Errorf("removing the folder: %d entries, want %d", len(plan), want)
	}
	if _, err := PlanRemove(scan(t), "pdf", "nowhere"); err == nil {
		t.Error("an unknown location was accepted")
	}
	if _, err := PlanRemove(scan(t), "pdf", "gemini"); err == nil {
		t.Error("a location the skill is not in was accepted")
	}
}

func TestInstallForOneAgent(t *testing.T) {
	home := machine(t, "claude", "codex")
	src := write(t, home, "pdf", "x")
	if _, err := InstallFrom(context.Background(), src, InstallOptions{To: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	s := skill(t, "pdf")
	if len(s.Installs) != 1 || s.Installs[0].Location != "claude" || strings.Join(s.Hidden, ",") != "codex" {
		t.Errorf("installs %+v, hidden %v", s.Installs, s.Hidden)
	}
	if _, err := InstallFrom(context.Background(), src, InstallOptions{To: []string{"nobody"}}); err == nil {
		t.Error("an unknown agent was accepted")
	}
}

func TestParseSource(t *testing.T) {
	home := machine(t)
	os.MkdirAll(filepath.Join(home, "skills", "pdf"), 0o755)
	for src, want := range map[string]Origin{
		"anthropics/skills":                                         {Git: "https://github.com/anthropics/skills"},
		"https://github.com/anthropics/skills.git":                  {Git: "https://github.com/anthropics/skills.git"},
		"git@github.com:anthropics/skills.git":                      {Git: "git@github.com:anthropics/skills.git"},
		"https://github.com/anthropics/skills/tree/main/skills/pdf": {Git: "https://github.com/anthropics/skills", Ref: "main", Subdir: "skills/pdf"},
		"https://github.com/anthropics/skills/tree/v2":              {Git: "https://github.com/anthropics/skills", Ref: "v2"},
		"~/skills/pdf": {Dir: filepath.Join(home, "skills", "pdf")},
	} {
		got, err := parseSource(src)
		want.Source = src
		if err != nil || got != want {
			t.Errorf("%s: %+v, %v", src, got, err)
		}
	}
	for _, src := range []string{"", "--upload-pack=x", "not a source", "/no/such/folder/anywhere"} {
		if _, err := parseSource(src); err == nil {
			t.Errorf("%q was accepted", src)
		}
	}
}

// A repository: installed from its URL, updated when it has a new commit.
func TestInstallFromGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	home := machine(t, "claude")
	repo := filepath.Join(home, "repo")
	write(t, repo, "pdf", "Reads PDFs")
	// A link in a repository is never followed: it could point anywhere.
	os.WriteFile(filepath.Join(home, "secret"), []byte("secret"), 0o600)
	_ = os.Symlink(filepath.Join(home, "secret"), filepath.Join(repo, "pdf", "leak"))
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("add", "-A")
	run("commit", "-q", "-m", "one")
	url := "file://" + filepath.ToSlash(repo)
	if !strings.HasPrefix(repo, "/") {
		url = "file:///" + filepath.ToSlash(repo)
	}
	ctx := context.Background()

	if _, err := InstallFrom(ctx, url, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	s := skill(t, "pdf")
	if s.Origin == nil || s.Origin.Git != url || s.Origin.Subdir != "pdf" || len(s.Origin.Commit) < 40 {
		t.Fatalf("origin: %+v", s.Origin)
	}
	dir := filepath.Join(home, ".claude", "skills", "pdf")
	for _, name := range []string{".git", "leak"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			t.Errorf("%s was copied", name)
		}
	}
	if res, err := Update(ctx, nil, false, false); err != nil || res[0].State != Current {
		t.Fatalf("no new commit: %+v, %v", res, err)
	}
	os.WriteFile(filepath.Join(repo, "pdf", "references", "notes.md"), []byte("v2\n"), 0o644)
	run("commit", "-q", "-am", "two")
	if res, _ := Update(ctx, nil, true, false); res[0].State != Available {
		t.Fatalf("check: %+v", res)
	}
	if res, _ := Update(ctx, nil, false, false); res[0].State != Updated {
		t.Fatalf("update: %+v", res)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "references", "notes.md"))
	if string(got) != "v2\n" {
		t.Errorf("after the update the file holds %q", got)
	}
	if res, _ := Update(ctx, nil, false, false); res[0].State != Current {
		t.Errorf("a second update: %+v", res)
	}
}
