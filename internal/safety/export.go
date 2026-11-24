package safety

import "strings"

// What other packages share with the policy, so that a command is cut into
// programs, and quoted for the shell, one way only.

// Segments splits a command line into its simple commands and their words
// the way the policy reads them: at ;, |, &, new lines, redirections,
// subshells and substitutions, with quotes and backslashes removed.
func Segments(cmd string) [][]string { return shellSegments(cmd) }

// AfterWrappers drops what only runs another program (sudo, env, nice,
// command, exec, xargs, a shell with -c, assignments, if/then/do) from the
// front of a simple command: the first word left is the program that runs.
func AfterWrappers(words []string) []string { return afterWrappers(words) }

// ShellQuote puts a word in single quotes for a POSIX shell, where nothing
// is expanded.
func ShellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
