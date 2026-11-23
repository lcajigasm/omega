package safety

import (
	"regexp"
	"strings"
)

// The catalog's checks are written as small scripts: echo "x: $(cmd)",
// for loops, while read, find with \( … \). Each piece only reads, so the
// whole should too; otherwise the agent, refused nothing but asked for
// approval, rewrites them one command at a time. What follows lets those
// shapes through when every command inside them is read-only, and keeps out
// the ways they could smuggle a change: a substitution or a loop variable
// that becomes an option (find . $(echo -delete)), and a loop that never
// ends (while true).

const (
	// substAny stands in for a substitution whose output cannot change
	// what the command does; substPrint for one that is only safe as text
	// to print.
	substAny   = "omegasubstany"
	substPrint = "omegasubstprint"
)

// tokenPrinters print a single word that cannot start with a dash, so
// their output may go anywhere in a command: gui/$(id -u)/x.
var tokenPrinters = regexp.MustCompile(`^\s*(id\s+-[ugnr]+|whoami|uname(\s+-[rmsnp])?|hostname\s+-s|brew\s+--(prefix|cellar|repo|cache)(\s+[\w@.+-]+)?|xcode-select\s+-p|date\s+\+[\w%:.-]+|go\s+env\s+[A-Z]+|npm\s+(root|prefix)\s+-g)\s*$`)

// substitutions replaces each $(…) in code with a placeholder when the
// command inside is read-only. ok is false for any other: a substitution
// that changes something, one that cannot be told apart from arithmetic,
// or one that is not closed. inners are the commands found.
func substitutions(code string) (out string, inners []string, ok bool) {
	var b strings.Builder
	var quote byte
	wordStart := 0
	for i := 0; i < len(code); i++ {
		c := code[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			}
		case c == '\\' && i+1 < len(code):
			b.WriteByte(c)
			i++
			c = code[i]
		case c == '$' && i+1 < len(code) && code[i+1] == '(':
			if i+2 < len(code) && code[i+2] == '(' {
				return "", nil, false // $((…)) with more than numbers in it
			}
			end := closingParen(code, i+2)
			if end < 0 {
				return "", nil, false
			}
			inner := code[i+2 : end]
			if Classify(inner) != Safe || AlwaysConfirm(inner) {
				return "", nil, false
			}
			inners = append(inners, inner)
			// Its output is split into words unless it is in double quotes,
			// and a word it starts can be an option.
			prefix := strings.NewReplacer(`"`, "", `'`, "").Replace(code[wordStart:i])
			midWord := quote == '"' && prefix != "" && !strings.HasPrefix(prefix, "-")
			if tokenPrinters.MatchString(inner) || midWord {
				b.WriteString(substAny)
			} else {
				b.WriteString(substPrint)
			}
			i = end
			continue
		case quote == '"':
			if c == '"' {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case strings.IndexByte(" \t\n;|&", c) >= 0:
			wordStart = i + 1
		}
		b.WriteByte(c)
	}
	out = b.String()
	if !strings.Contains(out, substPrint) {
		return out, inners, true
	}
	// A substitution whose words could be options is only text when the
	// command prints it.
	for _, part := range splitCommands(out) {
		if !strings.Contains(part, substPrint) {
			continue
		}
		words := dropKeywords(shellWords(part))
		for len(words) > 1 && strings.Contains(words[0].text, "=") {
			words = words[1:]
		}
		if len(words) == 0 || (words[0].text != "echo" && words[0].text != "printf") || strings.Contains(words[0].raw, substPrint) {
			return "", nil, false
		}
	}
	return out, inners, true
}

// closingParen finds the ) that closes a $( opened just before start,
// across quotes and nested parentheses. -1 when there is none.
func closingParen(s string, start int) int {
	depth := 1
	var quote byte
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			}
		case c == '\\' && i+1 < len(s):
			i++
		case quote == '"':
			if c == '"' {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// keywords open or continue a compound command; what follows them is a
// command of its own.
const keywords = "do then else elif if !"

func dropKeywords(words []word) []word {
	for len(words) > 0 && has(keywords, words[0].text) {
		words = words[1:]
	}
	return words
}

// classifyCompound decides the words of a compound command: the header of
// a for or while loop, its end, and what follows a keyword. ok is false
// for an ordinary command.
func classifyCompound(words []word) (Risk, bool) {
	if len(words) == 0 {
		return Safe, false
	}
	switch words[0].text {
	case "done", "fi":
		return Safe, len(words) == 1
	case "for":
		// for NAME [in WORD…]: it only sets a variable; the words are judged
		// with the rest of the line, and their use by loopVarsMisused.
		if len(words) < 2 || !plainVar.MatchString(words[1].text) || (len(words) > 2 && words[2].text != "in") {
			return Dangerous, true
		}
		return Safe, true
	case "while", "until":
		// Only "while read" ends by itself, at the end of its input.
		if len(words) > 1 && words[0].text == "while" && words[1].text == "read" {
			return classifyRead(words[2:]), true
		}
		return Dangerous, true
	case "read":
		return classifyRead(words[1:]), true
	}
	return Safe, false
}

// classifyRead reads a line into variables; it changes nothing else.
func classifyRead(args []word) Risk {
	for i := 0; i < len(args); i++ {
		a := args[i].text
		switch {
		case has("-d -n -N -t -u -p -a -A", a):
			i++
		case shortFlags.MatchString(a) && strings.Trim(a, "-rsea") == "":
		case !plainVar.MatchString(a):
			return Dangerous
		}
	}
	return Safe
}

// readNames are the variables a read sets.
func readNames(args []word) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i].text
		switch {
		case has("-d -n -N -t -u -p -a -A", a):
			i++
		case strings.HasPrefix(a, "-"):
		default:
			out = append(out, a)
		}
	}
	return out
}

var varAtStart = regexp.MustCompile(`^"?\$\{?([A-Za-z_][A-Za-z0-9_]*)`)

// literalWord is a loop value that cannot become an option or expand into
// other words: no dash in front, no variable, no substitution, and a
// pattern only when it is an absolute path (/x/*, ~/x/*), whose matches
// start with a slash.
func literalWord(w word) bool {
	t := strings.ReplaceAll(w.text, inWord, " ")
	if strings.HasPrefix(t, "-") || strings.ContainsAny(w.raw, "$`") {
		return false
	}
	if strings.ContainsAny(t, "*?[") && !strings.HasPrefix(t, "/") && !strings.HasPrefix(t, "~") {
		return false
	}
	return true
}

// loopVarsMisused reports a loop variable whose value is not known to be
// harmless used where it could be an option: "for f in *; do find . $f;
// done" deletes when a file is called -delete. Values are harmless when
// they are literal words, or lines printed by a loop over literal words.
// echo, printf and test only show or compare them.
func loopVarsMisused(code string, inners []string) bool {
	parts, seps := splitSeps(code)
	untrusted := map[string]bool{}
	literal := map[string]bool{}
	var stack []bool // per open loop: has it printed only literal words?
	lastLiteral := false
	prevSep := ""
	for i, part := range parts {
		words := shellWords(part)
		header := ""
		for len(words) > 0 && (has(keywords, words[0].text) || words[0].text == "while" || words[0].text == "until") {
			header = words[0].text
			words = words[1:]
		}
		for len(words) > 1 && strings.Contains(words[0].text, "=") && !strings.HasPrefix(words[0].text, "-") {
			words = words[1:]
		}
		switch {
		case len(words) == 0:
		case words[0].text == "for":
			ok := len(words) > 2 && words[2].text == "in"
			if ok {
				for _, w := range words[3:] {
					ok = ok && literalWord(w)
				}
			}
			if len(words) > 1 {
				name := words[1].text
				if ok {
					literal[name] = true
				} else {
					untrusted[name] = true
				}
			}
			stack = append(stack, true)
		case words[0].text == "read":
			// zsh runs the last stage of a pipeline in the current shell, so
			// "echo -delete | read x" sets x for the commands after it too.
			loop := header == "while" || header == "until"
			trusted := loop && prevSep == "|" && lastLiteral
			for _, n := range readNames(words[1:]) {
				if trusted {
					literal[n] = true
				} else {
					untrusted[n] = true
				}
			}
			if loop {
				stack = append(stack, true)
			}
		case words[0].text == "done":
			lastLiteral = false
			if n := len(stack); n > 0 {
				lastLiteral = stack[n-1]
				stack = stack[:n-1]
			}
		default:
			verb := words[0].text
			shows := verb == "echo" || verb == "printf"
			tests := verb == "[" || verb == "[[" || verb == "test"
			prints := shows
			for _, w := range words[1:] {
				m := varAtStart.FindStringSubmatch(w.raw)
				if m != nil && untrusted[m[1]] && !shows && !tests {
					return true
				}
				if shows && !(literalWord(w) || (m != nil && literal[m[1]] && !untrusted[m[1]] && strings.Trim(w.raw, `"`) == "$"+m[1])) {
					prints = false
				}
			}
			if n := len(stack); n > 0 && !tests && !prints {
				stack[n-1] = false
			}
			lastLiteral = prints
		}
		prevSep = seps[i]
	}
	for _, in := range inners {
		for v := range untrusted {
			if regexp.MustCompile(`\$\{?` + v + `\b`).MatchString(in) {
				return true
			}
		}
	}
	return false
}

// splitSeps is splitCommands with the separator after each part ("" after
// the last).
func splitSeps(cmd string) (parts, seps []string) {
	var cur strings.Builder
	var quote byte
	cut := func(sep string) {
		parts = append(parts, cur.String())
		seps = append(seps, sep)
		cur.Reset()
	}
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
			cur.WriteByte(c)
		case c == '\'' || c == '"':
			quote = c
			cur.WriteByte(c)
		case c == ';' || c == '\n':
			cut(string(c))
		case c == '&' && i+1 < len(cmd) && cmd[i+1] == '&', c == '|' && i+1 < len(cmd) && cmd[i+1] == '|':
			cut(cmd[i : i+2])
			i++
		case c == '|':
			cut("|")
		case c == '&' && ((i > 0 && cmd[i-1] == '>') || (i+1 < len(cmd) && cmd[i+1] == '>')):
			cur.WriteByte(c)
		case c == '&':
			cut("&")
		default:
			cur.WriteByte(c)
		}
	}
	cut("")
	return parts, seps
}

// findGroup is find's \( and \) written as words of their own.
var findGroup = regexp.MustCompile(`(^|\s)\\[()](\s|$)`)

// withoutFindGroups removes find's escaped parentheses, which the shell
// hands to find as plain words. Each \( must open a test (a word starting
// with a dash): PowerShell reads a backslash as an ordinary character, and
// there "\( Remove-Item x \)" would run what is inside. Its unary operators
// (-not, -bnot, -split, -join) could start an expression, so they do not
// count. ok is false when a \( does not open a test.
func withoutFindGroups(bare string) (string, bool) {
	f := strings.Fields(bare)
	for i, w := range f {
		if w == `\(` && (i+1 >= len(f) || !strings.HasPrefix(f[i+1], "-") || has("-not -bnot -split -join", strings.ToLower(f[i+1]))) {
			return bare, false
		}
	}
	for findGroup.MatchString(bare) {
		bare = findGroup.ReplaceAllString(bare, "$1$2")
	}
	return bare, true
}
