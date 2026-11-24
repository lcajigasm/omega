// Command omega is an AI-driven maintenance tool for macOS that combines what
// topgrade (update everything) and Mole (clean and optimize) do.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/lcajigasm/omega/internal/cli"
	"github.com/lcajigasm/omega/internal/sudo"
)

// Set by the release build via -ldflags.
var version = "dev"

func main() {
	// sudo runs this binary as its askpass helper; see internal/sudo.
	if sudo.IsHelper(os.Args) {
		os.Exit(sudo.RunHelper(os.Args))
	}
	// Exit status: 0 the run worked, 1 something failed, 2 warnings or
	// outdated packages (--fail-on-warning, --fail-on-outdated), 64 a command
	// line omega cannot use (EX_USAGE). An error that carries its status and
	// no text was already shown.
	if err := cli.Execute(version); err != nil {
		var code interface{ ExitCode() int }
		if errors.As(err, &code) {
			if msg := err.Error(); msg != "" {
				fmt.Fprintln(os.Stderr, "omega:", msg)
			}
			os.Exit(code.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "omega:", err)
		os.Exit(1)
	}
}
