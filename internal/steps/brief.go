package steps

import (
	"regexp"
	"strings"
)

// helperDef matches the start of one of omega's own shell helpers, such as
// `omega_space() {`, which the built-in steps define in front of the
// command they wrap.
var helperDef = regexp.MustCompile(`omega_[A-Za-z0-9_]+\(\) \{`)

// spaceGuard is the call that stops a step when the disk is too full.
var spaceGuard = regexp.MustCompile(`omega_space '[^']*' \|\| exit 1; `)

// Brief is a step's command as a person reads it in a list: omega's own
// helper functions and the free-space guard in front of it left out, so
// `brew update …` shows instead of a screen of awk. The command that runs
// is unchanged; a command with nothing to leave out comes back as it is.
func Brief(command string) string {
	if !strings.Contains(command, "omega_") {
		return command
	}
	out := command
	for {
		loc := helperDef.FindStringIndex(out)
		if loc == nil {
			break
		}
		end := closeBrace(out, loc[1])
		if end < 0 {
			break
		}
		rest := strings.TrimLeft(out[end:], " ")
		rest = strings.TrimPrefix(rest, ";")
		out = out[:loc[0]] + strings.TrimLeft(rest, " ")
	}
	out = spaceGuard.ReplaceAllString(out, "")
	out = strings.TrimSpace(out)
	if out == "" {
		return command
	}
	return out
}

// closeBrace returns the index just past the brace that closes the one
// opened before i, skipping braces inside quotes; -1 when it never closes.
func closeBrace(s string, i int) int {
	depth := 1
	var quote byte
	for ; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '\\':
			i++
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}
