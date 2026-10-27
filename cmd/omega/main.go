// Command omega is an AI-driven maintenance tool for macOS that combines what
// topgrade (update everything) and Mole (clean and optimize) do.
package main

import (
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
		fmt.Fprintln(os.Stderr, "omega:", err)
		os.Exit(1)
	}
}
