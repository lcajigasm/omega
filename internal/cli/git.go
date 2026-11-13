package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/gitops"
)

func gitCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "git",
		Short: "Deep Git repository hygiene and optimization",
		Long:  "Audit git repositories across development folders, find unpushed work, merged branches and optimize .git size.",
	}

	c.AddCommand(
		&cobra.Command{
			Use:   "status",
			Short: "Scan repositories for uncommitted changes, unpushed commits and merged branches",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				roots := gitops.DefaultRoots()
				fmt.Fprintf(cmd.OutOrStdout(), "\n🔍 Scanning dev directories for git repositories (%s)...\n", strings.Join(roots, ", "))
				repos := gitops.FindRepos(roots)
				if len(repos) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "  No git repositories found.")
					return nil
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Found %d repositories.\n\n", len(repos))
				ctx := context.Background()

				var dirtyCount, unpushedCount, mergedCount, uncheckedCount int

				for _, r := range repos {
					st, err := gitops.CheckRepo(ctx, r)
					if err != nil {
						// Not checked is not clean: say so.
						uncheckedCount++
						fmt.Fprintf(cmd.OutOrStdout(), "📁 %s  (%s)\n   ⚠ %v\n\n", st.Name, st.Path, err)
						continue
					}
					hasIssues := st.HasUncommitted || st.UnpushedCount > 0 || len(st.MergedBranches) > 0 || len(st.LargeFiles) > 0
					if !hasIssues {
						continue
					}

					fmt.Fprintf(cmd.OutOrStdout(), "📁 %s  (%s, .git: %d MB)\n", st.Name, st.Path, st.GitSizeMB)
					if st.HasUncommitted {
						dirtyCount++
						fmt.Fprintln(cmd.OutOrStdout(), "   ⚠ Uncommitted changes pending")
					}
					if st.UnpushedCount > 0 {
						unpushedCount++
						fmt.Fprintf(cmd.OutOrStdout(), "   ⚠ %d unpushed commit(s) ahead of remote\n", st.UnpushedCount)
					}
					if len(st.MergedBranches) > 0 {
						mergedCount += len(st.MergedBranches)
						fmt.Fprintf(cmd.OutOrStdout(), "   🌱 Merged local branches: %s\n", strings.Join(st.MergedBranches, ", "))
					}
					if len(st.LargeFiles) > 0 {
						fmt.Fprintf(cmd.OutOrStdout(), "   📦 Large files (>50MB): %s\n", strings.Join(st.LargeFiles, ", "))
					}
					fmt.Fprintln(cmd.OutOrStdout())
				}

				fmt.Fprintf(cmd.OutOrStdout(), "Summary: %d repos checked. %d with uncommitted work, %d with unpushed commits, %d merged branches.\n",
					len(repos)-uncheckedCount, dirtyCount, unpushedCount, mergedCount)
				if uncheckedCount > 0 {
					return fmt.Errorf("%d of %d repositories could not be checked", uncheckedCount, len(repos))
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "clean",
			Short: "Delete local merged branches across repositories",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				roots := gitops.DefaultRoots()
				repos := gitops.FindRepos(roots)
				ctx := context.Background()
				cleanedTotal, failedTotal := 0, 0

				for _, r := range repos {
					st, err := gitops.CheckRepo(ctx, r)
					if err != nil {
						fmt.Fprintf(cmd.OutOrStdout(), "  ⚠ %s: %v\n", r, err)
						continue
					}
					if len(st.MergedBranches) == 0 {
						continue
					}
					fmt.Fprintf(cmd.OutOrStdout(), "Repository %s has merged branches: %s\n", st.Name, strings.Join(st.MergedBranches, ", "))
					if askYesNo("Delete these merged local branches?") {
						deleted, err := gitops.DeleteMergedBranches(ctx, r, st.MergedBranches)
						if len(deleted) > 0 {
							fmt.Fprintf(cmd.OutOrStdout(), "  ✓ Deleted: %s\n", strings.Join(deleted, ", "))
						}
						if err != nil {
							failedTotal += len(st.MergedBranches) - len(deleted)
							fmt.Fprintf(cmd.OutOrStdout(), "  ⚠ Not deleted:\n    %s\n", strings.ReplaceAll(err.Error(), "\n", "\n    "))
						}
						cleanedTotal += len(deleted)
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\nDone! Cleaned %d merged branches.\n", cleanedTotal)
				if failedTotal > 0 {
					return fmt.Errorf("%d merged branches could not be deleted", failedTotal)
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "gc",
			Short: "Run git gc to pack loose objects and shrink .git directories",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				roots := gitops.DefaultRoots()
				repos := gitops.FindRepos(roots)
				ctx := context.Background()
				var totalFreed int64

				fmt.Fprintf(cmd.OutOrStdout(), "Optimizing %d git repositories...\n", len(repos))
				for _, r := range repos {
					freed, err := gitops.OptimizeRepo(ctx, r)
					if err != nil {
						fmt.Fprintf(cmd.OutOrStdout(), "  ⚠ %s: skipped (%v)\n", r, err)
						continue
					}
					if freed > 0 {
						fmt.Fprintf(cmd.OutOrStdout(), "  ✓ %s: freed %d MB\n", r, freed)
						totalFreed += freed
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Optimization complete. Total reclaimed space: %d MB\n", totalFreed)
				return nil
			},
		},
	)

	return c
}
