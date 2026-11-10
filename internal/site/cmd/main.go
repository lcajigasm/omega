// Command site writes the website under docs/ from site/. `make site` runs it.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/lcajigasm/omega/internal/site"
)

func main() {
	src := flag.String("src", "site", "templates and string tables")
	out := flag.String("out", "docs", "where the pages are written")
	flag.Parse()
	files, err := site.Build(*src)
	if err != nil {
		fmt.Fprintln(os.Stderr, "site:", err)
		os.Exit(1)
	}
	var paths []string
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		dest := filepath.Join(*out, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "site:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(dest, []byte(files[path]), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "site:", err)
			os.Exit(1)
		}
		fmt.Println(dest)
	}
}
