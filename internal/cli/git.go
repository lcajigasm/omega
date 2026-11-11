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

				var dirtyCount, unpushedCount, mergedCount int

				for _, r := range repos {
					st, err := gitops.CheckRepo(ctx, r)
					if err != nil {
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
					len(repos), dirtyCount, unpushedCount, mergedCount)
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
				cleanedTotal := 0

				for _, r := range repos {
					st, err := gitops.CheckRepo(ctx, r)
					if err != nil || len(st.MergedBranches) == 0 {
						continue
					}
					fmt.Fprintf(cmd.OutOrStdout(), "Repository %s has merged branches: %s\n", st.Name, strings.Join(st.MergedBranches, ", "))
					ans := ask("Delete these merged local branches? [y/N]", "n")
					if ans == "y" || ans == "Y" {
						deleted, _ := gitops.DeleteMergedBranches(ctx, r, st.MergedBranches)
						fmt.Fprintf(cmd.OutOrStdout(), "  ✓ Deleted: %s\n", strings.Join(deleted, ", "))
						cleanedTotal += len(deleted)
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\nDone! Cleaned %d merged branches.\n", cleanedTotal)
				return nil
			},
		},
		&cobra.Command{
			Use:   "gc",
			Short: "Run git gc and aggressive repacking to shrink .git directories",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				roots := gitops.DefaultRoots()
				repos := gitops.FindRepos(roots)
				ctx := context.Background()
				var totalFreed int64

				fmt.Fprintf(cmd.OutOrStdout(), "Optimizing %d git repositories...\n", len(repos))
				for _, r := range repos {
					freed, err := gitops.OptimizeRepo(ctx, r)
					if err == nil && freed > 0 {
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
