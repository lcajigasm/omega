package safety

import (
	"regexp"
	"strings"
)

// Syntax that hides what a command runs from a reader of its text.

// uncPath is a Windows network path (\\host\share): merely listing one
// makes Windows sign in to that host with the user's NTLM hash.
var uncPath = regexp.MustCompile(`(^|[\s'"=(,;|&])\\\\[^\\\s'"]+\\`)

// escapedBreak is a backslash before a character that ends a command or
// opens a quote. The POSIX shells and PowerShell read it in opposite ways
// (in sh, \; is a literal semicolon and \" a literal quote; in PowerShell
// the backslash is an ordinary character), so what follows cannot be split
// into commands with any confidence.
var escapedBreak = regexp.MustCompile("\\\\[;|&\"'`\n\r]")

// hidesCommands reports syntax that can carry a command where the splitter
// does not see one:
//
//   - an escaped quote or separator: echo \"; rm -rf x; echo \" is three
//     commands for the shell and one quoted word for a naive reader;
//   - $'…', whose quoting rules differ from '…';
//   - PowerShell's block comment <# … #>, which a POSIX reader does not end
//     where PowerShell does.
func hidesCommands(cmd string) bool {
	return escapedBreak.MatchString(cmd) || ansiCQuote(cmd) || strings.Contains(cmd, "<#") || strings.Contains(cmd, "#>")
}

// ansiCQuote reports a $'…' word. A $ just before the closing quote of an
// ordinary string ('^ 0$', a regular expression's end of line) is not one.
func ansiCQuote(cmd string) bool {
	var quote byte
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '$' && i+1 < len(cmd) && cmd[i+1] == '\'':
			return true
		case c == '\'' || c == '"':
			quote = c
		}
	}
	return false
}

// unquoted is the command without what is inside its quotes.
func unquoted(cmd string) string {
	var out strings.Builder
	var quote byte
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		default:
			out.WriteByte(c)
		}
	}
	return out.String()
}
