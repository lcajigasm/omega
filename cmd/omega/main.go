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
	if err := cli.Execute(version); err != nil {
		var code interface{ ExitCode() int }
		if errors.As(err, &code) {
			os.Exit(code.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "omega:", err)
		os.Exit(1)
	}
}
