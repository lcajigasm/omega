package safety

import (
	"regexp"
	"strings"
)

// The policy reads a command as text; the shell reads it as words, after
// removing comments, quotes and backslashes and expanding ${…} and {a,b}.
// What follows brings the two readings together: the checks on options
// look at the words the program receives, and a command whose words cannot
// be known from its text is not read-only.

// stripComments removes the comments of a command line: an unquoted # that
// starts a word hides the rest of the line from the shell, so it must hide
// it from the policy too. Otherwise a quote inside a comment swallows the
// command on the next line.
func stripComments(cmd string) string {
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
		case c == '#' && (i == 0 || strings.IndexByte(" \t\r\n;|&(", cmd[i-1]) >= 0):
			for i < len(cmd) && cmd[i] != '\n' {
				i++
			}
			if i < len(cmd) {
				out.WriteByte('\n')
			}
			continue
		}
		out.WriteByte(c)
	}
	return out.String()
}

// inWord stands in for a blank inside a quoted word, and alone for an empty
// word, so that a word with spaces stays one field for the checks.
const inWord = "\x1f"

// word is one word of a command: as it was typed and as the program gets it.
type word struct {
	raw  string
	text string
}

// shellWords splits one simple command into words the way a POSIX shell
// does: blanks (spaces, tabs, newlines) separate, quotes and backslashes are
// removed.
func shellWords(part string) []word {
	var out []word
	var raw, text strings.Builder
	started := false
	flush := func() {
		if !started {
			return
		}
		t := text.String()
		if t == "" {
			t = inWord
		}
		out = append(out, word{raw: raw.String(), text: t})
		raw.Reset()
		text.Reset()
		started = false
	}
	put := func(c byte) {
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			text.WriteString(inWord)
			return
		}
		text.WriteByte(c)
	}
	var quote byte
	for i := 0; i < len(part); i++ {
		c := part[i]
		switch {
		case quote == '\'':
			raw.WriteByte(c)
			if c == '\'' {
				quote = 0
			} else {
				put(c)
			}
		case quote == '"':
			raw.WriteByte(c)
			switch {
			case c == '"':
				quote = 0
			case c == '\\' && i+1 < len(part) && strings.IndexByte("\"\\$`", part[i+1]) >= 0:
				i++
				raw.WriteByte(part[i])
				put(part[i])
			default:
				put(c)
			}
		case c == '\'' || c == '"':
			quote = c
			started = true
			raw.WriteByte(c)
		case c == '\\' && i+1 < len(part):
			started = true
			raw.WriteByte(c)
			i++
			raw.WriteByte(part[i])
			put(part[i])
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			flush()
		default:
			started = true
			raw.WriteByte(c)
			text.WriteByte(c)
		}
	}
	flush()
	return out
}

func joinText(words []word) string {
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = w.text
	}
	return strings.Join(parts, " ")
}

func joinRaw(words []word) string {
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = w.raw
	}
	return strings.Join(parts, " ")
}

var (
	plainVar = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*|[0-9@*#?$!-])$`)
	// zsh applies modifiers and subscripts to a bare $name: $x:s/a/b/, $x[1,3].
	// PowerShell's $env:NAME has the same shape and is only a variable.
	varModifier = regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_]*(:[A-Za-z]|\[)`)
	psEnvVar    = regexp.MustCompile(`(?i)\$env:`)
)

// outsideSingle is the command without what is inside single quotes, where
// the shell expands nothing.
func outsideSingle(s string) string {
	var out strings.Builder
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else if quote == '"' {
				out.WriteByte(c)
			}
		case c == '\'' || c == '"':
			quote = c
		default:
			out.WriteByte(c)
		}
	}
	return out.String()
}

// rewritesWords reports a command whose words the shell builds while it
// runs, so the text does not say what the program receives:
//
//   - ${VAR:+-delete}, ${VAR/a/b}, ${VAR:0:3}: anything in ${…} but a name;
//   - $VAR:s/a/b/ and $VAR[1,3], which zsh also expands;
//   - an option whose dash is quoted or escaped ("-delete", -\delete,
//     -"delete"), or written as a pattern (-[d]elete): nobody writes one
//     that way except to get it past a reader.
//
// A plain $VAR or ${VAR} is not reported. Brace expansion is: see bracesOK.
func rewritesWords(words []word) bool {
	if len(words) == 0 {
		return false
	}
	live := outsideSingle(joinRaw(words))
	for rest := live; ; {
		i := strings.Index(rest, "${")
		if i < 0 {
			break
		}
		rest = rest[i+2:]
		j := strings.IndexByte(rest, '}')
		if j < 0 || !plainVar.MatchString(rest[:j]) {
			return true
		}
	}
	if varModifier.MatchString(psEnvVar.ReplaceAllString(live, "")) {
		return true
	}
	// echo and printf have no option that does anything: echo "- done".
	prints := words[0].text == "echo" || words[0].text == "printf"
	for _, w := range words[1:] {
		if !strings.HasPrefix(w.text, "-") || w.text == "-" {
			continue
		}
		name, _, _ := strings.Cut(w.raw, "=")
		if strings.ContainsAny(unquoted(name), "*?[") {
			return true
		}
		// The dash itself quoted, or a quote right after it.
		if first := strings.TrimLeft(w.raw, "-"); !prints && first != "" && strings.IndexByte(`'"\`, first[0]) >= 0 {
			return true
		}
	}
	return false
}

// gitRef is what git writes between braces: HEAD@{1}, @{u}, stash@{0}.
var gitRef = regexp.MustCompile(`^[\w.-]*$`)

// bracesOK reports that every brace outside quotes is one of the few that
// change nothing: find's {} placeholder, a git reference such as @{u}, a
// ${VAR}, or a PowerShell block that only reads a property of the current
// object. Any other is brace expansion ({a,b} makes two words out of one)
// or a script block, which is code.
func bracesOK(raw string) bool {
	var quote byte
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
			continue
		case c == '\'' || c == '"':
			quote = c
			continue
		case c != '{':
			continue
		}
		end := -1
		var q byte
		for j := i + 1; j < len(raw) && end < 0; j++ {
			switch d := raw[j]; {
			case q != 0:
				if d == q {
					q = 0
				}
			case d == '\'' || d == '"':
				q = d
			case d == '{':
				return false // nested
			case d == '}':
				end = j
			}
		}
		if end < 0 {
			return false
		}
		body := raw[i+1 : end]
		prev := byte(0)
		if i > 0 {
			prev = raw[i-1]
		}
		switch {
		case prev == '$': // ${…} is judged by rewritesWords
		case body == "":
		case prev == '@' && gitRef.MatchString(body):
		case psSimpleBlock.MatchString(body):
		default:
			return false
		}
		i = end
	}
	return true
}
