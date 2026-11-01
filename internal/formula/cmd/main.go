// Command formula rewrites Formula/omega.rb from a sha256sum file.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/lcajigasm/omega/internal/formula"
)

func main() {
	version := flag.String("version", "", "release version, x.y.z, without the v prefix")
	sumsPath := flag.String("sums", "", "path to SHA256SUMS")
	file := flag.String("file", "Formula/omega.rb", "formula to rewrite")
	flag.Parse()
	if *version == "" || *sumsPath == "" {
		fmt.Fprintln(os.Stderr, "usage: formula -version 0.10.0 -sums dist/SHA256SUMS [-file Formula/omega.rb]")
		os.Exit(2)
	}
	raw, err := os.ReadFile(*sumsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sums, err := formula.ParseSums(string(raw))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	src, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out, err := formula.Update(string(src), *version, sums)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(*file, []byte(out), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
