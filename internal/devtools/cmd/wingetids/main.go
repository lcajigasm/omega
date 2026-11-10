// Command wingetids prints the winget ids omega dev installs with, one per
// line, for the CI check that each still exists.
package main

import (
	"fmt"

	"github.com/lcajigasm/omega/internal/devtools"
)

func main() {
	for _, id := range devtools.WingetIDs() {
		fmt.Println(id)
	}
}
