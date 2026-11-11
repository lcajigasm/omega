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
			r.Register("update", func(ctx context.Context, args []string) error {
				return runDeterministic(agent.TaskUpdate, runFlags{yes: true})
			})
			r.Register("clean", func(ctx context.Context, args []string) error {
				return runDeterministic(agent.TaskClean, runFlags{yes: true})
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
				sub := "flush"
				if len(args) > 0 {
					sub = args[0]
				}
				if sub == "flush" {
					return netCmd().Commands()[0].RunE(nil, nil)
				}
				return netCmd().Commands()[1].RunE(nil, nil)
			})
			r.Register("audit", func(ctx context.Context, args []string) error {
				return auditCmd().Commands()[0].RunE(nil, nil)
			})
			r.Register("privacy", func(ctx context.Context, args []string) error {
				return privacyCmd().RunE(nil, nil)
			})
			r.Register("status", func(ctx context.Context, args []string) error {
				return runDoctor(os.Stdout)
			})

			fmt.Println()
			return r.Run(context.Background())
		},
	}
}
