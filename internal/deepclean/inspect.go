package deepclean

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Inspect gathers the evidence about one finding a careful person would
// look at before removing it: for a worktree, every file git reports, the
// last commits, the size of the diff and, with gh, the state of the pull
// request of its branch; for agent files, each file with its size; for
// Docker, what the object is. It only reads.
func Inspect(ctx context.Context, f Finding) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\nverdict: %s\naction: %s\n", f.Title, f.Verdict, f.Action)
	for _, r := range f.Reasons {
		fmt.Fprintf(&b, "- %s\n", r)
	}
	for _, r := range f.Rescue {
		fmt.Fprintf(&b, "- to rescue first: %s\n", r)
	}
	section := func(title string, out string, err error) {
		fmt.Fprintf(&b, "\n## %s\n", title)
		switch {
		case err != nil:
			fmt.Fprintf(&b, "(could not read: %v)\n", err)
		case strings.TrimSpace(out) == "":
			b.WriteString("(nothing)\n")
		default:
			b.WriteString(clip(strings.TrimRight(out, "\n"), 60) + "\n")
		}
	}
	switch f.Kind {
	case KindWorktree:
		out, err := command(ctx, f.Path, "git", "status", "--short", "--untracked-files=all")
		section("git status (every file)", out, err)
		out, err = command(ctx, f.Path, "git", "diff", "--stat", "HEAD")
		section("diff against HEAD", out, err)
		out, err = command(ctx, f.Path, "git", "log", "-8", "--format=%h %cr %an: %s")
		section("last commits", out, err)
		if f.Ref != "" && !strings.HasPrefix(f.Ref, "detached") {
			out, err = command(ctx, f.Repo, "git", "branch", "-a", "--contains", "HEAD", "--format=%(refname:short)")
			section("branches that contain its HEAD", out, err)
			if has("gh") {
				out, err = command(ctx, f.Path, "gh", "pr", "list", "--head", f.Ref, "--state", "all", "--limit", "5",
					"--json", "number,state,title,mergedAt,updatedAt", "--template",
					`{{range .}}#{{.number}} {{.state}} {{.title}} (updated {{.updatedAt}}{{if .mergedAt}}, merged {{.mergedAt}}{{end}}){{"\n"}}{{end}}`)
				section("pull requests of branch "+f.Ref+" (gh)", out, err)
			}
		}
		out, err = command(ctx, f.Path, "git", "stash", "list")
		section("stashes of the repository (they stay)", out, err)
	case KindAgentScratch:
		var lines []string
		for _, name := range scratchFiles(f.Path) {
			if fi, err := os.Stat(filepath.Join(f.Path, name)); err == nil {
				lines = append(lines, fmt.Sprintf("%s  %d bytes  %s", name, fi.Size(), fi.ModTime().Format("2006-01-02 15:04")))
			}
		}
		section("files", strings.Join(lines, "\n"), nil)
	case KindDockerImage:
		out, err := command(ctx, "", "docker", "image", "inspect", "--format",
			"tags: {{join .RepoTags \", \"}}\ndigests: {{join .RepoDigests \", \"}}\ncreated: {{.Created}}\nlabels: {{json .Config.Labels}}", f.Ref)
		section("image", out, err)
	case KindDockerContainer:
		out, err := command(ctx, "", "docker", "container", "inspect", "--format",
			"image: {{.Config.Image}}\nstatus: {{.State.Status}} (exit {{.State.ExitCode}})\nmounts: {{range .Mounts}}{{.Type}}:{{.Name}}{{.Source}}->{{.Destination}} {{end}}", f.Ref)
		section("container", out, err)
		out, err = command(ctx, "", "docker", "container", "diff", f.Ref)
		section("what it changed inside itself", out, err)
	case KindDockerVolume:
		out, err := command(ctx, "", "docker", "volume", "inspect", "--format",
			"driver: {{.Driver}}\ncreated: {{.CreatedAt}}\nlabels: {{json .Labels}}", f.Ref)
		section("volume", out, err)
	}
	return b.String()
}

// clip keeps the first n lines.
func clip(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n") + fmt.Sprintf("\n… and %d more lines", len(lines)-n)
}
