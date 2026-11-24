package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/safety"
)

// run_command and the shell under every tool that runs a command: the
// approval, the output the model sees, and what a command's output says
// about the next step.

func (a *Agent) runShell(ctx context.Context, tool, command, reason string, risk safety.Risk, always bool) (string, error) {
	if out, ok := a.cached(tool, command, risk); ok {
		return out, nil
	}
	if ok, why := a.authorize(ctx, tool, command, reason, risk, always); !ok {
		return "", explained(errors.New(why))
	}
	if risk > safety.Safe {
		// Something may change: what was measured before may no longer hold.
		a.forget()
	}
	id := a.nextCall()
	a.emit(Event{Kind: EvToolStart, Tool: tool, Command: command, Reason: reason, Risk: risk, call: id})
	lines, full := 0, 0
	run, stop := context.WithCancel(ctx)
	defer stop()
	res := a.opts.Exec(run, command, a.opts.Timeout, func(line string) {
		lines++
		a.emit(Event{Kind: EvOutput, Tool: tool, Command: command, Text: maskPersonal(command, line), call: id})
		// A full disk does not get better by itself: brew went on for two
		// minutes and 13,000 failed files after the first one.
		if strings.Contains(line, "No space left on device") {
			if full++; full == diskFullLines {
				stop()
			}
		}
	})
	if full >= diskFullLines && ctx.Err() == nil {
		res.Output += fmt.Sprintf("\nomega stopped this command: the disk is full (%d \"No space left on device\" errors). "+
			"Free space first (measure with system_info and disk_usage, clean caches), then run it again.", full)
		if res.ExitCode == 0 {
			res.ExitCode = 1
		}
	}
	a.emit(Event{Kind: EvToolEnd, Tool: tool, Command: command, ExitCode: res.ExitCode, Failed: res.ExitCode != 0, Duration: res.Duration, Lines: lines, call: id})
	// A step that ran out of time or was cancelled (exit -1) did not fail
	// in a way a stronger model can fix: it would only cost more.
	stopped := res.ExitCode == -1 || res.TimedOut || ctx.Err() != nil
	if res.ExitCode != 0 && !stopped && strings.HasPrefix(tool, "run_step:") && !benignFailure(res.Output) {
		a.escalate(fmt.Sprintf("%s failed with exit code %d", tool, res.ExitCode))
	}
	if asksConfirmation.MatchString(res.Output) {
		a.noteRefusal(command)
	}
	out := strings.TrimSpace(collapseRepeats(maskPersonal(command, res.Output)))
	if out == "" {
		out = "(no output)"
	}
	if res.TimedOut {
		out += "\n(stopped: the command ran out of time)"
	}
	out += followUpHint(res.Output)
	result := fmt.Sprintf("exit_code: %d\nduration: %s\n---\n%s", res.ExitCode, res.Duration.Round(time.Second), out)
	if risk == safety.Safe && res.ExitCode == 0 {
		a.remember(tool, command, result)
	}
	return result, nil
}

// diskFullLines is how many "No space left on device" errors stop a command.
const diskFullLines = 20

// asksConfirmation is a program saying it wants a person to confirm.
var asksConfirmation = regexp.MustCompile(`(?i)requires? confirmation|--yes to confirm|use --yes|re-?run with (--yes|-y|--force|-f)\b|pass (--yes|--force)|confirm with (--yes|-y)`)

// forceFlag adds a confirmation on a person's behalf.
var forceFlag = regexp.MustCompile(`(^|\s)(--yes|-y|--force|-f|--assume-yes|--no-confirm|--noconfirm)(\s|=|$)`)

// programKey names a program and its subcommand: "mo purge", "brew cleanup".
// The wrappers before it (sudo, env, assignments) are skipped the way the
// safety policy skips them.
func programKey(words []string) string {
	words = safety.AfterWrappers(words)
	if len(words) == 0 {
		return ""
	}
	key := filepath.Base(words[0])
	if len(words) > 1 && !strings.HasPrefix(words[1], "-") {
		key += " " + words[1]
	}
	return key
}

// commandSegments are the simple commands of a line and their words, as
// the safety policy reads them: quotes do not hide a separator.
func commandSegments(cmd string) [][]string { return safety.Segments(cmd) }

// noteRefusal records the programs of a command whose output asked for a
// person's confirmation.
func (a *Agent) noteRefusal(cmd string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.refusedConfirm == nil {
		a.refusedConfirm = map[string]bool{}
	}
	for _, seg := range commandSegments(cmd) {
		if k := programKey(seg); k != "" {
			a.refusedConfirm[k] = true
		}
	}
}

// overridesRefusal reports a command that adds --yes or --force to a
// program that asked for a person's confirmation earlier in this session.
func (a *Agent) overridesRefusal(cmd string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, seg := range commandSegments(cmd) {
		if a.refusedConfirm[programKey(seg)] && forceFlag.MatchString(strings.Join(seg, " ")) {
			return true
		}
	}
	return false
}

// settingsLink finds a command that only opens a pane of System Settings.
var settingsLink = regexp.MustCompile(`^\s*open\s+['"]?(x-apple\.systempreferences:[^'"\s;&|]*)['"]?`)

// Masks for configuration that names people and machines: the hosts, users
// and key files of ~/.ssh/config, and e-mail addresses in git's settings.
var (
	sshValue = regexp.MustCompile(`(?i)^(\s*(?:HostName|User|IdentityFile|CertificateFile|ProxyJump|ProxyCommand|LocalForward|RemoteForward)\s+).+$`)
	email    = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
)

// maskPersonal hides, in what a command printed, the parts of ~/.ssh/config
// and of git's configuration that identify people and servers. The aliases
// and the options stay: they are what maintenance needs.
func maskPersonal(command, text string) string {
	lower := strings.ToLower(command)
	if strings.Contains(lower, ".ssh/config") || strings.Contains(lower, "ssh -g") {
		lines := strings.Split(text, "\n")
		for i, l := range lines {
			lines[i] = sshValue.ReplaceAllString(l, "${1}<hidden>")
		}
		text = strings.Join(lines, "\n")
	}
	if strings.Contains(lower, "git config") || strings.Contains(lower, "gitconfig") || strings.Contains(lower, ".ssh/config") {
		text = email.ReplaceAllString(text, "<email>")
	}
	return text
}

func toolRunCommand(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	cmd := getString(in, "command")
	if cmd == "" {
		return "", fmt.Errorf("empty command")
	}
	// The card shows the command; the shell runs its bytes. A command that
	// carries an escape sequence, a carriage return or a mark that reorders
	// or hides text can read as one thing and run as another, so it is
	// sent back to be written plainly.
	if r, ok := hiddenRune(cmd); ok {
		msg := fmt.Sprintf("the command contains a control or invisible character (U+%04X), which can make it read differently from what runs; write it with printable characters only", r)
		a.emit(Event{Kind: EvDenied, Tool: "run_command", Command: visible(cmd), Reason: msg})
		return "", explained(errors.New(msg))
	}
	if m := settingsLink.FindStringSubmatch(cmd); m != nil && a.opts.Unattended {
		why := "Unattended run: nobody is at the screen, so System Settings was not opened. Put this link in your summary " +
			"for the user to open: " + m[1]
		a.emit(Event{Kind: EvDenied, Tool: "run_command", Command: visible(cmd), Reason: why})
		return "", explained(errors.New(why))
	}
	risk := safety.Classify(cmd)
	always := safety.AlwaysConfirm(cmd) || (risk > safety.Safe && sensitiveCommand(cmd))
	if a.overridesRefusal(cmd) {
		// The program asked for a person; --yes from the model is not one.
		always = true
		if risk < safety.Dangerous {
			risk = safety.Dangerous
		}
	}
	out, err := a.runShell(ctx, "run_command", cmd, visible(getString(in, "reason")), risk, always)
	if err == nil && fetchesNetwork(cmd) {
		// What a server sent is as untrusted as a page fetch_url reads.
		a.markTainted()
	}
	return out, err
}

// networkTools download what a server sends: their output is web content.
const networkTools = "curl wget wget2 http https httpie xh aria2c fetch lynx w3m links elinks invoke-webrequest iwr invoke-restmethod irm"

// fetchesNetwork reports a command that prints what a server sent.
func fetchesNetwork(cmd string) bool {
	for _, seg := range safety.Segments(cmd) {
		if run := safety.AfterWrappers(seg); len(run) > 0 {
			verb := strings.TrimSuffix(strings.ToLower(filepath.Base(run[0])), ".exe")
			if strings.Contains(" "+networkTools+" ", " "+verb+" ") || (verb == "gh" && len(run) > 1 && run[1] == "api") {
				return true
			}
		}
	}
	return false
}

// hiddenRune finds a character a person cannot see for what it is: a
// control character other than a line break or a tab, or one that formats
// the text around it (the marks that turn text right to left, zero-width
// characters, the Unicode line separators).
func hiddenRune(s string) (rune, bool) {
	for _, r := range s {
		if r == '\n' || r == '\t' {
			continue
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' || r == utf8.RuneError {
			return r, true
		}
	}
	return 0, false
}

// visible writes those characters as what they are (<U+202E>), for text
// that is shown to the user.
func visible(s string) string {
	if _, ok := hiddenRune(s); !ok {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if _, hidden := hiddenRune(string(r)); hidden {
			fmt.Fprintf(&b, "<U+%04X>", r)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// sensitiveCommand reports a command that changes what later runs without
// anyone asking, the way sensitiveWrite does for write_file: it names a
// shell startup file, a login item, a scheduled job, omega's own settings
// or notes, or it runs omega itself (omega config set approval never,
// omega schedule install, omega full --unattended). Such a command is asked
// about every time, also under --yes and after "approve all": those say yes
// to maintenance, not to a new rule for what the next run may do.
func sensitiveCommand(cmd string) bool {
	home, _ := os.UserHomeDir()
	words := strings.FieldsFunc(cmd, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(`;|&<>()='"`+"`", r)
	})
	for _, w := range words {
		w = strings.NewReplacer("${HOME}", home, "$HOME", home, "$env:USERPROFILE", home).Replace(w)
		w = expand(w)
		if w == "" || (!strings.ContainsAny(w, `/\`) && !strings.HasPrefix(w, ".")) {
			continue
		}
		if sensitiveWrite(w) || filepath.Clean(w) == filepath.Dir(config.Dir()) {
			return true
		}
	}
	for _, seg := range safety.Segments(cmd) {
		if f := safety.AfterWrappers(seg); len(f) > 0 {
			name := strings.ToLower(filepath.Base(f[0]))
			if name == "omega" || name == "omega.exe" || name == "omega-gui" {
				return true
			}
		}
	}
	return false
}

// benignFailures are failures a stronger model cannot do anything about:
// a daemon that is not running, a tool that is missing or has nothing to
// update, a warning, a disk that is full, administrator access not given.
// Switching to the expensive model for them only costs money.
var benignFailures = []string{
	"Cannot connect to the Docker daemon", "Is the docker daemon running", "command not found", "not installed",
	"no repositories found", "No global packages", "Nothing to upgrade", "No space left on device",
	"a password is required", "Failed to elevate", "admin access not granted", "Please note that these warnings",
	"Your system is ready to brew", "only free", "free space first",
}

func benignFailure(output string) bool {
	for _, s := range benignFailures {
		if strings.Contains(output, s) {
			return true
		}
	}
	return false
}

// followUpHint points the model at the next sensible offer for well-known
// failures, so it suggests a fix instead of just reporting them.
func followUpHint(output string) string {
	switch {
	case strings.Contains(output, "Cannot connect to the Docker daemon") || strings.Contains(output, "Is the docker daemon running"):
		return "\n\nNOTE: Docker Desktop is not running. Offer to start it (step docker-start) with propose_actions, " +
			"then measure with docker-df before offering any Docker cleanup."
	case strings.Contains(output, "command not found") || strings.Contains(output, "No such file or directory: "):
		return "\n\nNOTE: a tool is missing. If it is needed for what the user asked, offer to install it " +
			"(usually brew install <formula>, see list_steps) with propose_actions. Never install without a selection."
	case strings.Contains(output, "a password is required") || strings.Contains(output, "admin access not granted") ||
		strings.Contains(output, "requires sudo"):
		return "\n\nNOTE: this needed administrator access that was not granted. Do not retry it in this run; " +
			"mention in the summary that it can be done with omega --sudo."
	}
	return ""
}
