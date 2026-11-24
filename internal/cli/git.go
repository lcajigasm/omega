package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/gitops"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/ui"
)

// branchesShown is how many merged branches of a repository git status
// lists before "and N more".
const branchesShown = 10

func gitCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "git",
		Short: i18n.T("cmd.git"),
		Long:  i18n.T("cmd.git_long", strings.Join(gitRootNames(), ", ")),
		Example: "  omega git status\n" +
			"  omega git status --all --json\n" +
			"  omega git clean --dry-run\n" +
			"  omega git clean\n" +
			"  omega git gc",
	}

	var asJSON, all bool
	status := &cobra.Command{
		Use:   "status",
		Short: i18n.T("cmd.git_status"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signalContext()
			defer stop()
			scan := scanRepos(ctx, cmd.ErrOrStderr())
			if asJSON {
				return writeGitJSON(cmd.OutOrStdout(), scan)
			}
			return writeGitStatus(cmd.OutOrStdout(), scan, all)
		},
	}
	status.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.tools_json"))
	status.Flags().BoolVar(&all, "all", false, i18n.T("flag.git_all"))

	var yes, dryRun bool
	clean := &cobra.Command{
		Use:     "clean",
		Short:   i18n.T("cmd.git_clean"),
		Long:    i18n.T("cmd.git_clean_long"),
		Example: "  omega git clean --dry-run\n  omega git clean\n  omega git clean --yes",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signalContext()
			defer stop()
			return cleanBranches(ctx, cmd.OutOrStdout(), cmd.ErrOrStderr(), yes, dryRun)
		},
	}
	clean.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("flag.tools_yes"))
	clean.Flags().BoolVarP(&dryRun, "dry-run", "n", false, i18n.T("flag.tools_dry"))

	gc := &cobra.Command{
		Use:   "gc",
		Short: i18n.T("cmd.git_gc"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signalContext()
			defer stop()
			repos := gitops.FindRepos(gitops.DefaultRoots())
			w := cmd.OutOrStdout()
			r := ui.NewReport(w)
			r.Title("omega git gc", i18n.N("git.repos_found", len(repos)))
			r.Blank()
			p := newProgress(cmd.ErrOrStderr())
			var total int64
			for i, repo := range repos {
				if ctx.Err() != nil {
					break
				}
				p.Force(fmt.Sprintf("[%d/%d] %s…", i+1, len(repos), tildePath(repo)))
				freed, err := gitops.OptimizeRepo(ctx, repo)
				p.Clear()
				if err != nil {
					r.Check(ui.MarkWarn, tildePath(repo), 0, i18n.T("git.gc_skipped", err))
					continue
				}
				if freed > 0 {
					r.Check(ui.MarkOK, tildePath(repo), 0, i18n.T("git.gc_freed", ui.Bytes(freed)))
					total += freed
				}
			}
			r.Blank()
			r.Check(ui.MarkOK, i18n.T("git.gc_total", ui.Bytes(total)), 0, "")
			if ctx.Err() != nil {
				r.Hint(i18n.T("tools.interrupted"))
			}
			return nil
		},
	}

	c.AddCommand(status, clean, gc)
	return c
}

// gitRootNames are the folders looked through, as the user writes them.
func gitRootNames() []string {
	var out []string
	for _, r := range gitops.DefaultRoots() {
		out = append(out, tildePath(r))
	}
	if len(out) == 0 {
		return []string{"~/Dev", "~/Projects", "~/Code", "~/src"}
	}
	return out
}

// repoScan is what git status and git clean found.
type repoScan struct {
	Roots       []string             `json:"roots"`
	Repos       []*gitops.RepoStatus `json:"repos"`
	Unchecked   []repoError          `json:"unchecked,omitempty"`
	Total       int                  `json:"total"`
	Interrupted bool                 `json:"interrupted,omitempty"`
}

type repoError struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// scanRepos checks every repository under the default roots, saying which
// one it is on in progress (on a terminal). Ctrl-C stops it and keeps what
// was checked.
func scanRepos(ctx context.Context, progressTo io.Writer) repoScan {
	p := newProgress(progressTo)
	roots := gitops.DefaultRoots()
	scan := repoScan{Roots: roots, Repos: []*gitops.RepoStatus{}}
	p.Force(i18n.T("git.finding"))
	repos := gitops.FindRepos(roots)
	scan.Total = len(repos)
	for i, repo := range repos {
		if ctx.Err() != nil {
			scan.Interrupted = true
			break
		}
		p.Force(fmt.Sprintf("[%d/%d] %s…", i+1, len(repos), tildePath(repo)))
		st, err := gitops.CheckRepo(ctx, repo)
		if err != nil {
			if ctx.Err() != nil {
				scan.Interrupted = true
				break
			}
			scan.Unchecked = append(scan.Unchecked, repoError{Path: repo, Error: err.Error()})
			continue
		}
		scan.Repos = append(scan.Repos, st)
	}
	p.Clear()
	return scan
}

func writeGitJSON(w io.Writer, scan repoScan) error {
	if err := writeJSON(w, scan); err != nil {
		return err
	}
	if len(scan.Unchecked) > 0 {
		return exitCode(1)
	}
	return nil
}

// mergedInto is the branch merged branches were measured against.
func mergedInto(st *gitops.RepoStatus) string {
	if st.DefaultBranch == "" {
		return "HEAD"
	}
	return st.DefaultBranch
}

// listBranches is "a, b, c" with at most branchesShown names unless all.
func listBranches(branches []string, all bool) string {
	if all || len(branches) <= branchesShown {
		return strings.Join(branches, ", ")
	}
	return strings.Join(branches[:branchesShown], ", ") + " " + i18n.T("git.and_more", len(branches)-branchesShown)
}

func writeGitStatus(w io.Writer, scan repoScan, all bool) error {
	r := ui.NewReport(w)
	roots := make([]string, 0, len(scan.Roots))
	for _, root := range scan.Roots {
		roots = append(roots, tildePath(root))
	}
	r.Title("omega git status", strings.Join(roots, ", "))
	if scan.Total == 0 {
		r.Blank()
		r.Line(i18n.T("git.none_found"))
		return nil
	}
	var dirty, unpushed, merged, notDefault int
	for _, st := range scan.Repos {
		issues := st.HasUncommitted || st.UnpushedCount > 0 || len(st.MergedBranches) > 0 || len(st.LargeFiles) > 0
		if !issues {
			continue
		}
		detail := st.Branch
		if st.GitSize > 0 {
			detail += " · .git " + ui.Bytes(st.GitSize)
		}
		r.Section(st.Name, -1)
		r.Line(r.Muted(tildePath(st.Path) + " · " + detail))
		const lw = 12
		if st.HasUncommitted {
			dirty++
			r.Check(ui.MarkWarn, i18n.T("git.label_changes"), lw, i18n.T("git.uncommitted"))
		}
		if st.UnpushedCount > 0 {
			unpushed++
			r.Check(ui.MarkWarn, i18n.T("git.label_unpushed"), lw, i18n.T("git.unpushed", st.UnpushedCount))
		}
		if len(st.MergedBranches) > 0 {
			merged += len(st.MergedBranches)
			r.Check(ui.MarkInfo, i18n.T("git.label_merged", len(st.MergedBranches)), lw, listBranches(st.MergedBranches, all))
			if !st.OnDefault && st.DefaultBranch != "" {
				notDefault++
				r.Detail(i18n.T("git.not_default", st.Branch, st.DefaultBranch))
			}
		}
		if len(st.LargeFiles) > 0 {
			r.Check(ui.MarkWarn, i18n.T("git.label_large"), lw, listBranches(st.LargeFiles, all))
		}
	}
	if len(scan.Unchecked) > 0 {
		r.Section(i18n.T("git.unchecked_section"), len(scan.Unchecked))
		for _, u := range scan.Unchecked {
			r.Check(ui.MarkErr, tildePath(u.Path), 0, u.Error)
		}
	}
	r.Blank()
	r.Line(i18n.T("git.summary", len(scan.Repos), dirty, unpushed, merged))
	if scan.Interrupted {
		r.Hint(i18n.T("git.interrupted", len(scan.Repos)+len(scan.Unchecked), scan.Total))
	}
	if merged > 0 {
		r.Hint(i18n.T("git.clean_hint"))
	}
	if len(scan.Unchecked) > 0 {
		return fmt.Errorf("%s", i18n.T("git.unchecked", len(scan.Unchecked), scan.Total))
	}
	return nil
}

// cleanBranches finds the merged branches of every repository, shows them,
// and deletes the ones chosen in a single picker (all of them with --yes).
func cleanBranches(ctx context.Context, w, progressTo io.Writer, yes, dryRun bool) error {
	scan := scanRepos(ctx, progressTo)
	r := ui.NewReport(w)
	r.Title("omega git clean", i18n.N("git.repos_found", scan.Total))
	var withMerged []*gitops.RepoStatus
	count := 0
	for _, st := range scan.Repos {
		if len(st.MergedBranches) > 0 {
			withMerged = append(withMerged, st)
			count += len(st.MergedBranches)
		}
	}
	for _, u := range scan.Unchecked {
		r.Check(ui.MarkErr, tildePath(u.Path), 0, u.Error)
	}
	if len(withMerged) == 0 {
		r.Blank()
		r.Check(ui.MarkOK, i18n.T("git.no_merged"), 0, "")
		return nil
	}
	for _, st := range withMerged {
		r.Section(st.Name, len(st.MergedBranches))
		r.Line(r.Muted(tildePath(st.Path) + " · " + i18n.T("git.merged_into", mergedInto(st))))
		if !st.OnDefault && st.DefaultBranch != "" {
			r.Check(ui.MarkWarn, i18n.T("git.not_default", st.Branch, st.DefaultBranch), 0, "")
		}
		r.Line(listBranches(st.MergedBranches, dryRun))
	}
	r.Blank()
	if dryRun {
		r.Hint(i18n.T("git.dry_run", count, len(withMerged)))
		return nil
	}

	chosen := withMerged
	if !yes {
		if !stdinIsTerminal() {
			return errors.New(i18n.T("git.need_yes"))
		}
		var opts []ui.Option
		for i, st := range withMerged {
			opts = append(opts, ui.Option{
				ID:       fmt.Sprint(i),
				Label:    st.Name + " · " + i18n.T("git.label_merged", len(st.MergedBranches)),
				Detail:   r.Clip(strings.Join(st.MergedBranches, ", "), 10),
				Risk:     "normal",
				Selected: true,
			})
		}
		ids, err := ui.Pick(i18n.N("git.pick_title", count), i18n.T("git.pick_hint"), opts, true, true, os.Stdin, w)
		if errors.Is(err, ui.ErrCancelled) || (err == nil && len(ids) == 0) {
			fmt.Fprintln(w, i18n.T("tools.nothing_done"))
			return nil
		}
		if err != nil {
			return err
		}
		chosen = nil
		for _, id := range ids {
			var i int
			fmt.Sscan(id, &i)
			chosen = append(chosen, withMerged[i])
		}
	}

	deleted, failed := 0, 0
	for _, st := range chosen {
		if ctx.Err() != nil {
			break
		}
		done, err := gitops.DeleteMergedBranches(ctx, st.Path, st.MergedBranches)
		deleted += len(done)
		if len(done) > 0 {
			r.Check(ui.MarkOK, st.Name, 0, i18n.T("git.deleted", listBranches(done, false)))
		}
		if err != nil {
			failed += len(st.MergedBranches) - len(done)
			r.Check(ui.MarkWarn, st.Name, 0, i18n.T("git.not_deleted"))
			for _, line := range strings.Split(err.Error(), "\n") {
				r.Detail(line)
			}
		}
	}
	r.Blank()
	r.Line(i18n.N("git.clean_done", deleted))
	if failed > 0 {
		return fmt.Errorf("%s", i18n.N("git.clean_failed", failed))
	}
	return nil
}
