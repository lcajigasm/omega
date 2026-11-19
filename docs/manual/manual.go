// Package manual embeds omega's manual, so `omega docs` and the MCP server
// answer from the pages this binary was built with, offline.
package manual

import "embed"

// FS holds the manual's Markdown pages.
//
//go:embed *.md
var FS embed.FS
