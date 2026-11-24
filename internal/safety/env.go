package safety

import (
	"path"
	"regexp"
	"strings"
)

// The environment: what a command may print of it, and the variables set
// in front of a command, which can change what it runs.

// envDumpReshaped reports "env" or "printenv" printing every variable into
// something that can cut the values up. The credentials tools need stay in
// the environment (see KeptSecretVars) and are masked in the output by
// value: "env | grep -o ." prints them one letter per line, where no mask
// can match. Filters that keep whole lines are fine.
func envDumpReshaped(code string) bool {
	for _, pipeline := range splitUnquoted(code, false, true) {
		dumped := false
		for _, stage := range splitUnquoted(pipeline, true, false) {
			f := strings.Fields(joinText(shellWords(stage)))
			if len(f) == 0 {
				continue
			}
			if !dumped {
				dumped = dumpsEnv(f)
				continue
			}
			switch f[0] {
			case "sort", "uniq", "wc":
			case "grep", "egrep", "fgrep":
				for _, a := range f[1:] {
					if abbrev(a, "--only-matching") || (shortFlags.MatchString(a) && strings.Contains(a, "o")) {
						return true
					}
				}
			case "head", "tail":
				for _, a := range f[1:] {
					if abbrev(a, "--bytes") || (strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "c")) {
						return true
					}
				}
			default:
				return true
			}
		}
	}
	return false
}

// dumpsEnv reports printenv, or env with no command: what they print is
// the value of variables, one of which may be a credential.
func dumpsEnv(f []string) bool {
	if f[0] == "printenv" {
		return true
	}
	if f[0] != "env" {
		return false
	}
	for _, a := range f[1:] {
		if !strings.HasPrefix(a, "-") && !strings.Contains(a, "=") {
			return false
		}
	}
	return true
}

// envChangers set variables for the commands after them.
const envChangers = "export declare typeset local readonly setenv eval source . set unset"

// sudoEnvChanged reports a command that runs sudo after changing the
// environment in any way but the harmless variables. omega answers sudo
// through the helper SUDO_ASKPASS names; a command that points it at a
// program of its own, under any spelling the name check misses
// (export "SUDO_ASK""PASS"=x, declare -x, eval), would be handed the
// password by a sudo the user approved.
// quotedSegments splits a command into its simple commands and their words
// the way the shell does: what is inside quotes stays one word. A pattern
// such as grep '(pre|loop) *= *true' is not a subshell and an assignment.
// Subshells and substitutions are still their own segments.
func quotedSegments(cmd string) [][]string {
	var out [][]string
	for _, part := range splitCommands(cmd) {
		part = strings.TrimSpace(strings.Trim(strings.TrimSpace(part), "(){}"))
		if strings.Contains(part, "$(") || strings.Contains(part, "`") {
			// What runs inside a substitution is a command of its own.
			out = append(out, shellSegments(part)...)
			continue
		}
		var words []string
		for _, w := range shellWords(part) {
			if w.text != "" {
				words = append(words, w.text)
			}
		}
		if len(words) > 0 {
			out = append(out, words)
		}
	}
	return out
}

func sudoEnvChanged(cmd string) bool {
	segs := quotedSegments(cmd)
	sudo := false
	for _, seg := range segs {
		for _, w := range seg {
			b := path.Base(w)
			if b == "sudo" || b == "doas" {
				sudo = true
				break
			}
			if !strings.Contains(w, "=") && !has("env nice nohup time command exec builtin if then else elif do while until !", b) && !strings.HasPrefix(w, "-") {
				break
			}
		}
	}
	if !sudo {
		return false
	}
	for _, seg := range segs {
		for _, w := range seg {
			if strings.Contains(w, "=") && !strings.HasPrefix(w, "-") {
				if !harmlessVars.MatchString(w) {
					return true
				}
				continue
			}
			if has(envChangers, path.Base(w)) {
				return true
			}
			if path.Base(w) == "env" {
				continue // its assignments are checked as they come
			}
			break
		}
		// sudo's own way to set a variable for the command: sudo VAR=x.
		for i, w := range seg {
			if path.Base(w) == "sudo" {
				for _, a := range seg[i+1:] {
					if strings.Contains(a, "=") && !strings.HasPrefix(a, "-") && !harmlessVars.MatchString(a) {
						return true
					}
					if !strings.HasPrefix(a, "-") && !strings.Contains(a, "=") {
						break
					}
				}
			}
		}
	}
	return false
}

// harmlessVars are the variables a command may be given without changing
// what it runs. Any other assignment in front of a command is treated as a
// change: PATH picks another program, GIT_CONFIG_* and PAGER make a
// read-only tool run one.
//
// Between the two are plain settings (FOO=1, HOMEBREW_NO_AUTO_UPDATE=1 is
// already harmless): a name that does not choose a program, a path, a
// library or options, with a value that is a plain word. Such a command is
// judged by what follows it, and is never read-only: env FOO=1 brew
// upgrade is a routine upgrade, not a change of what runs.
var harmlessVars = regexp.MustCompile(`^(IFS|LANG|LANGUAGE|LC_[A-Z]+|TZ|TERM|COLUMNS|LINES|NO_COLOR|CLICOLOR|CLICOLOR_FORCE|CI|DEBIAN_FRONTEND|HOMEBREW_NO_[A-Z_]+|HOMEBREW_COLOR|NONINTERACTIVE)=[\w.:/@+-]*$`)

var (
	// riskyVarName is a variable that chooses a program, a library, a
	// folder or the options of a program: setting it changes what runs.
	riskyVarName = regexp.MustCompile(`(?i)^(PATH|IFS|ENV|BASH_ENV|ZDOTDIR|SHELL|HOME|CDPATH|FPATH|PROMPT_COMMAND|PS4|EDITOR|VISUAL|PAGER|BROWSER|TERMINFO|MANPAGER|SYSTEMD_PAGER|LESS\w*|LD_\w+|DYLD_\w+|GIT_\w+|SUDO_\w+|SSH_\w+|GPG_\w+|HOMEBREW_\w+|NODE_\w+|NPM_\w+|PYTHON\w*|PIP_\w+|PERL\w*|RUBY\w*|GEM_\w+|BUNDLE_\w+|CARGO_\w+|RUSTUP_\w+|RUSTC\w*|GO\w*|JAVA_\w+|_JAVA_OPTIONS|DOCKER_\w+|KUBE\w*|AWS_\w+|GOOGLE_\w+|AZURE_\w+|TMPDIR|XDG_\w+|OMEGA_\w*|\w*(_PATH|_COMMAND|_CMD|_PROGRAM|_EXEC|_HOOK|_HELPER|_SHELL|_EDITOR|_PAGER|_CONFIG|_FILE|_DIR|_HOME|_OPTS|_OPTIONS|_FLAGS|_ARGS|_PRELOAD|_LIBRARY\w*|PROXY|_URL|_HOST|_REMOTE|_REGISTRY|_ASKPASS|_TOKEN|_KEY|_SECRET|_PASSWORD))=`)
	plainValue   = regexp.MustCompile(`^[^=]+=[\w.:@+-]*$`)
)

// assignmentRisk is the least risk of a command run with this variable
// set: Safe for the harmless ones, Normal for a plain setting, Dangerous
// for one that can change what runs.
func assignmentRisk(kv string) Risk {
	switch {
	case harmlessVars.MatchString(kv):
		return Safe
	case riskyVarName.MatchString(kv) || !plainValue.MatchString(kv):
		return Dangerous
	}
	return Normal
}
