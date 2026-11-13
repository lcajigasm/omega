package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/runner"
)

func onSystem(t *testing.T, system, osRelease string, bins ...string) {
	t.Helper()
	oldOS, oldRel, oldHas := goos, readOSRelease, hasBin
	goos, readOSRelease = system, func() string { return osRelease }
	hasBin = func(bin string) bool {
		for _, b := range bins {
			if b == bin {
				return true
			}
		}
		return false
	}
	t.Cleanup(func() { goos, readOSRelease, hasBin = oldOS, oldRel, oldHas })
}

// official_docs answers with the references of the system omega runs on, and
// on Linux with those of the distribution and of the ones it derives from.
func TestOfficialDocsPerSystem(t *testing.T) {
	cases := []struct {
		system, osRelease string
		bins              []string
		want, never       []string
	}{
		{"darwin", "", []string{"brew", "git"},
			[]string{"support.apple.com/guide/mac-help", "docs.brew.sh", "git-scm.com", "man <command>"}, []string{"learn.microsoft.com", "archlinux", "winget"}},
		{"windows", "", []string{"winget", "wsl", "git"},
			[]string{"learn.microsoft.com/en-us/windows/package-manager/winget/", "wsl-config", "Get-Help"}, []string{"support.apple.com", "debian.org"}},
		{"linux", "ID=ubuntu\nID_LIKE=debian\n", []string{"apt-get", "snap"},
			[]string{"documentation.ubuntu.com", "debian.org/doc", "freedesktop.org", "snapcraft.io"}, []string{"archlinux", "fedoraproject", "support.apple.com"}},
		{"linux", "ID=\"rocky\"\nID_LIKE=\"rhel centos fedora\"\n", []string{"dnf"},
			[]string{"docs.fedoraproject.org", "dnf.readthedocs.io"}, []string{"debian.org", "archlinux"}},
		{"linux", "ID=opensuse-tumbleweed\nID_LIKE=\"opensuse suse\"\n", []string{"zypper"},
			[]string{"doc.opensuse.org", "man7.org"}, []string{"debian.org", "fedoraproject"}},
		{"linux", "ID=arch\n", []string{"pacman"}, []string{"wiki.archlinux.org/title/System_maintenance"}, []string{"debian.org"}},
	}
	for _, c := range cases {
		onSystem(t, c.system, c.osRelease, c.bins...)
		out, err := toolOfficialDocs(context.Background(), nil, map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("%s (%q): missing %q in\n%s", c.system, c.osRelease, w, out)
			}
		}
		for _, n := range c.never {
			if strings.Contains(out, n) {
				t.Errorf("%s (%q): %q does not belong in\n%s", c.system, c.osRelease, n, out)
			}
		}
	}
}

func TestOfficialDocsTopic(t *testing.T) {
	onSystem(t, "darwin", "", "brew", "git", "docker")
	out, _ := toolOfficialDocs(context.Background(), nil, map[string]any{"topic": "Docker"})
	if !strings.Contains(out, "docs.docker.com") || strings.Contains(out, "docs.brew.sh") {
		t.Errorf("a topic keeps only what matches:\n%s", out)
	}
	out, _ = toolOfficialDocs(context.Background(), nil, map[string]any{"topic": "snapshots"})
	if !strings.Contains(out, "disk-utility") {
		t.Errorf("a subject finds the system reference that covers it:\n%s", out)
	}
	// Nothing filed under the topic: say so, and still give the system's references.
	out, _ = toolOfficialDocs(context.Background(), nil, map[string]any{"topic": "ghostty"})
	if !strings.Contains(out, "web_search") || !strings.Contains(out, "support.apple.com") {
		t.Errorf("an unknown topic still points somewhere:\n%s", out)
	}
}

// Every reference is an https page with something to say about it, and the
// sites they live on are the ones a search result is marked official for.
func TestDocsCatalogue(t *testing.T) {
	for _, d := range platformDocs {
		if d.Name == "" || d.Covers == "" || len(d.URLs) == 0 || (d.OS != "darwin" && d.OS != "linux" && d.OS != "windows") {
			t.Errorf("incomplete reference: %+v", d)
		}
		for _, u := range d.URLs {
			if !strings.HasPrefix(u, "https://") {
				t.Errorf("%s: %s is not https", d.Name, u)
			}
		}
	}
	for _, h := range hints {
		if len(h.Docs) == 0 || h.Ideas == "" {
			t.Errorf("hint %s needs docs and ideas", h.Tool)
		}
		for _, u := range h.Docs {
			if !strings.HasPrefix(u, "https://") {
				t.Errorf("%s: %s is not https", h.Tool, u)
			}
		}
	}
	for link, want := range map[string]bool{
		"https://learn.microsoft.com/en-us/windows/wsl/basic-commands":         true,
		"https://support.apple.com/en-us/102154":                               true,
		"https://www.debian.org/releases/stable/":                              true,
		"https://wiki.archlinux.org/title/Pacman":                              true,
		"http://support.apple.com/en-us/102154":                                false, // not https
		"https://github.com/someone/dotfiles":                                  false, // anybody publishes there
		"https://support.apple.com.example.org/fix":                            false,
		"https://medium.com/@someone/10-mac-tweaks":                            false,
		"https://docs.brew.sh/Manpage":                                         true,
		"https://superuser.com/questions/1/how-to-speed-up-windows-in-one-key": false,
	} {
		if got := officialURL(link); got != want {
			t.Errorf("officialURL(%s) = %v, want %v", link, got, want)
		}
	}
}

// The tool is offered on every system, is read-only, and the prompt tells
// the model to work from the documentation.
func TestOfficialDocsOffered(t *testing.T) {
	for _, system := range []string{"darwin", "linux", "windows"} {
		onSystem(t, system, "")
		found := false
		for _, s := range toolSpecs() {
			found = found || s.Name == "official_docs"
		}
		if !found {
			t.Errorf("%s: official_docs is not offered", system)
		}
		if p := systemPrompt("", nil, false); !strings.Contains(p, "official_docs") || !strings.Contains(p, "not from memory") {
			t.Errorf("%s: the prompt does not send the model to the documentation", system)
		}
	}
	if !readOnlyCall(llm.ToolCall{Name: "official_docs"}) {
		t.Error("official_docs only reads")
	}
	if _, ok := handlers["official_docs"]; !ok {
		t.Error("official_docs has no handler")
	}
}

// An unattended run is upkeep: what the user arranged themselves (the Dock,
// the menu bar, shortcuts, their apps) is left as it is, whichever way the
// model tries to change it.
func TestUnattendedLeavesPersonalSetupAlone(t *testing.T) {
	onSystem(t, "darwin", "")
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "mac_change", map[string]any{"change": "dock-remove:com.apple.Maps", "reason": "unused"})}},
		{Calls: []llm.ToolCall{call("2", "mac_change", map[string]any{"change": "app-remove:com.example.Old", "reason": "unused"})}},
		{Calls: []llm.ToolCall{call("3", "mac_policy", map[string]any{"kind": "dock-remove", "value": "Maps"})}},
		{Calls: []llm.ToolCall{call("4", "run_command", map[string]any{"command": "defaults write com.apple.dock persistent-apps -array && killall Dock", "reason": "tidy"})}},
		{Calls: []llm.ToolCall{call("5", "run_command", map[string]any{"command": "dockutil --remove Maps", "reason": "tidy"})}},
		{Calls: []llm.ToolCall{call("6", "run_command", map[string]any{"command": "brew cleanup", "reason": "r"})}},
		{Text: "done"},
	}}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var ran []string
	a := New(p, Options{AutoYes: true, Unattended: true, Config: config.Default(), Approval: config.ApproveAll,
		Timeout: 5 * time.Second, Exec: func(_ context.Context, cmd string, _ time.Duration, _ func(string)) runner.Result {
			ran = append(ran, cmd)
			return runner.Result{}
		}})
	go a.Run(context.Background(), "test")
	denied := 0
	for e := range a.Events() {
		switch e.Kind {
		case EvDenied:
			denied++
		case EvChoice, EvConfirm, EvPassword, EvIdle:
			t.Fatalf("an unattended run asked something: %+v", e)
		}
	}
	for i := 0; i < 5; i++ {
		if r := p.results[i+1][0]; !r.IsError || !strings.Contains(r.Content, "Unattended run") {
			t.Errorf("call %d must be rejected: %+v", i+1, r)
		}
	}
	if denied != 5 {
		t.Errorf("denied %d, want 5", denied)
	}
	if strings.Join(ran, ";") != "brew cleanup" {
		t.Fatalf("ran %q", ran)
	}
}
