package devtools

import (
	"strings"
	"testing"
)

func TestWindowsInstallPrefersScoopThenWinget(t *testing.T) {
	old := Current
	t.Cleanup(func() { Current = old })
	tool := func(id string, p Platform) Tool {
		Current = func() Platform { return p }
		for _, x := range Tools() {
			if x.ID == id {
				return x
			}
		}
		t.Fatalf("no tool %s", id)
		return Tool{}
	}
	both := Platform{OS: "windows", Shell: "powershell", Winget: true, Scoop: true}
	if got := tool("fzf", both).Install; got != "scoop install fzf" {
		t.Errorf("with Scoop: %q", got)
	}
	onlyWinget := Platform{OS: "windows", Shell: "powershell", Winget: true}
	if got := tool("fzf", onlyWinget).Install; !strings.HasPrefix(got, "winget install --id junegunn.fzf --exact") || !strings.Contains(got, "--disable-interactivity") {
		t.Errorf("with winget only: %q", got)
	}
	// Scoop carries no Docker Desktop: winget installs it even when Scoop is there.
	if got := tool("docker", both).Install; !strings.HasPrefix(got, "winget install --id Docker.DockerDesktop") {
		t.Errorf("docker: %q", got)
	}
	// Neither manager: the tool is detected, not offered.
	if got := tool("fzf", Platform{OS: "windows", Shell: "powershell"}).Install; got != "" {
		t.Errorf("no manager: %q", got)
	}
	// Unix-only and Mac-only tools are never offered, and nothing needs a
	// line in a shell startup file.
	for _, id := range []string{"asdf", "nvm", "pyenv", "rbenv", "tmux", "cocoapods", "orbstack", "swift"} {
		if got := tool(id, both).Install; got != "" {
			t.Errorf("%s is offered on Windows: %q", id, got)
		}
	}
	for _, x := range Tools() {
		if x.Init != "" || x.Mark != "" {
			t.Errorf("%s wants a shell line on Windows", x.ID)
		}
		for _, posix := range []string{"brew ", "sudo ", "&&", "curl "} {
			if strings.Contains(x.Install, posix) {
				t.Errorf("%s: %q is not a Windows command", x.ID, x.Install)
			}
		}
	}
	if got := tool("python", both).Bin; got != "python" {
		t.Errorf("python is looked for as %q", got)
	}
	// Every Windows entry names a tool of the catalogue.
	known := map[string]bool{}
	for _, c := range Catalog {
		known[c.ID] = true
	}
	for id := range windowsTools {
		if !known[id] {
			t.Errorf("windowsTools has %q, which is not in the catalogue", id)
		}
	}
	if len(WingetIDs()) < 40 {
		t.Errorf("only %d winget ids", len(WingetIDs()))
	}
}
