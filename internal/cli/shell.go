package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/repl"
)

func shellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell",
		Short: "Start an interactive Omega maintenance REPL session",
		Long:  "Launch an interactive terminal console to run successive health checks, maintenance tasks, and diagnostics.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r := repl.New(os.Stdin, os.Stdout, version)

			// Wire commands
			r.Register("quick", func(ctx context.Context, args []string) error {
				return runDeterministic(agent.TaskQuick, runFlags{})
			})
			// No "yes" here: a dangerous step is asked about, as it is on
			// the command line.
			r.Register("update", func(ctx context.Context, args []string) error {
				return runDeterministic(agent.TaskUpdate, runFlags{})
			})
			r.Register("clean", func(ctx context.Context, args []string) error {
				return runDeterministic(agent.TaskClean, runFlags{})
			})
			r.Register("analyze", func(ctx context.Context, args []string) error {
				return runDeterministic(agent.TaskAnalyze, runFlags{})
			})
			r.Register("health", func(ctx context.Context, args []string) error {
				return runDeterministic(agent.TaskHealth, runFlags{})
			})
			r.Register("doctor", func(ctx context.Context, args []string) error {
				return runDoctor(os.Stdout)
			})
			r.Register("net", func(ctx context.Context, args []string) error {
				if len(args) == 0 {
					args = []string{"audit"}
				}
				return runSub(ctx, netCmd(), args)
			})
			r.Register("audit", func(ctx context.Context, args []string) error {
				if len(args) == 0 {
					args = []string{"ssh"}
				}
				return runSub(ctx, auditCmd(), args)
			})
			r.Register("privacy", func(ctx context.Context, args []string) error {
				return runSub(ctx, privacyCmd(), args)
			})
			r.Register("dev", func(ctx context.Context, args []string) error {
				if len(args) == 0 {
					args = []string{"status"}
				}
				return runSub(ctx, devCmd(), args)
			})
			r.Register("steps", func(ctx context.Context, args []string) error {
				return runSub(ctx, stepsCmd(), args)
			})
			r.Register("status", func(ctx context.Context, args []string) error {
				return runDoctor(os.Stdout)
			})

			fmt.Println()
			return r.Run(context.Background())
		},
	}
}

// runSub runs a command of omega from inside the shell, the way the command
// line would: cobra finds the subcommand by name, checks the arguments and
// hands the handler a real command to write to.
func runSub(ctx context.Context, c *cobra.Command, args []string) error {
	if args == nil {
		args = []string{}
	}
	c.SetArgs(args)
	c.SetOut(os.Stdout)
	c.SetErr(os.Stderr)
	c.SilenceUsage = true
	c.SilenceErrors = true
	return c.ExecuteContext(ctx)
}
