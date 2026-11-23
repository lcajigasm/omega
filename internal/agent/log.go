package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/config"
)

// MaxRunLogs is how many transcripts omega keeps under RunsDir.
const MaxRunLogs = 30

// RunsDir is the directory of run transcripts (~/.local/state/omega/runs).
func RunsDir() string { return filepath.Join(config.StateDir(), "runs") }

// RunLog is one transcript on disk.
type RunLog struct {
	Name    string
	Path    string
	Size    int64
	ModTime time.Time
}

// ListRunLogs returns transcripts newest first. A missing directory is empty.
func ListRunLogs() ([]RunLog, error) {
	entries, err := os.ReadDir(RunsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []RunLog
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, RunLog{
			Name:    e.Name(),
			Path:    filepath.Join(RunsDir(), e.Name()),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out, nil
}

// ReadRunLog reads a transcript by list index (1 is the newest), file name, or
// the newest when name is empty.
func ReadRunLog(name string) (RunLog, []byte, error) {
	logs, err := ListRunLogs()
	if err != nil {
		return RunLog{}, nil, err
	}
	pick, ok := pickRunLog(logs, name)
	if !ok {
		return RunLog{}, nil, os.ErrNotExist
	}
	b, err := os.ReadFile(pick.Path)
	return pick, b, err
}

func pickRunLog(logs []RunLog, name string) (RunLog, bool) {
	if name == "" {
		if len(logs) == 0 {
			return RunLog{}, false
		}
		return logs[0], true
	}
	if n, err := strconv.Atoi(name); err == nil {
		if n >= 1 && n <= len(logs) {
			return logs[n-1], true
		}
		return RunLog{}, false
	}
	base := filepath.Base(name)
	if !strings.HasSuffix(base, ".log") {
		base += ".log"
	}
	for _, l := range logs {
		if l.Name == base {
			return l, true
		}
	}
	return RunLog{}, false
}

// rotateRunLogs deletes the oldest transcripts until at most keep remain.
// protect is never removed, so the file just opened stays put.
func rotateRunLogs(dir, protect string, keep int) {
	if keep < 1 {
		keep = 1
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	excess := len(names) - keep
	for _, name := range names {
		if excess <= 0 {
			break
		}
		path := filepath.Join(dir, name)
		if path == protect {
			continue
		}
		if os.Remove(path) == nil {
			excess--
		}
	}
}

func (a *Agent) openLog(provider string) {
	dir := RunsDir()
	// The transcripts hold every command's output: for this user only.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	_ = os.Chmod(dir, 0o700) // a folder made by an earlier version
	// Two runs can start in the same second (the desktop app and a
	// scheduled one): the second takes the next free name, never the
	// first one's file.
	stamp := time.Now().Format("2006-01-02_150405")
	path := filepath.Join(dir, stamp+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	for n := 2; errors.Is(err, os.ErrExist) && n < 100; n++ {
		path = filepath.Join(dir, fmt.Sprintf("%s_%d.log", stamp, n))
		f, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	}
	if err != nil {
		return
	}
	a.log = f
	// The first line keeps its old shape: the app reads the title from it.
	fmt.Fprintf(f, "omega run %s — %s / %s\n", time.Now().Format(time.RFC3339), provider, a.opts.Model)
	f.WriteString(a.logHeader())
	f.WriteString("\n")
	rotateRunLogs(dir, path, MaxRunLogs)
}

// logHeader is what a transcript needs to be read later: which omega, on
// which machine, started how, and under which rules. Two machines can
// share one folder of transcripts, and a run that was unattended reads very
// differently from one somebody watched.
func (a *Agent) logHeader() string {
	var b strings.Builder
	version := a.opts.Version
	if version == "" {
		version = "dev"
	}
	fmt.Fprintf(&b, "version: %s · host: %s\n", version, hostSummary(a.real))
	if a.opts.Command != "" || a.opts.Task != "" {
		fmt.Fprintf(&b, "command: %s", visible(a.opts.Command))
		if a.opts.Task != "" {
			fmt.Fprintf(&b, " (task: %s)", a.opts.Task)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "mode: %s\n", a.modeSummary())
	return b.String()
}

// modeSummary names, in one line, who can answer and what is allowed.
func (a *Agent) modeSummary() string {
	var parts []string
	switch {
	case a.opts.Unattended:
		parts = append(parts, "unattended (nobody answers; options are taken automatically, actions that need a person are skipped)")
	case a.opts.OneShot:
		parts = append(parts, "guided stage (a person answers cards; the session ends with the turn)")
	case a.opts.Interactive:
		parts = append(parts, "interactive")
	default:
		parts = append(parts, "non-interactive (nobody answers cards)")
	}
	switch {
	case a.opts.ReadOnly:
		parts = append(parts, "read-only")
	case a.opts.AutoYes:
		parts = append(parts, "automatic approval (--yes)")
	}
	approval := a.opts.Approval
	if approval == "" {
		approval = "default"
	}
	parts = append(parts, "approval: "+approval)
	if a.opts.SudoPassword != "" {
		parts = append(parts, "sudo: granted (--sudo)")
	} else {
		parts = append(parts, "sudo: asked when needed")
	}
	return strings.Join(parts, " · ")
}

// hostSummary is the machine's name, model and disk, for the header. Tests
// and the demo do not describe the machine they run on.
func hostSummary(real bool) string {
	if !real {
		return "test"
	}
	name, _ := os.Hostname()
	parts := []string{name}
	if m := hardwareModel(); m != "" {
		parts = append(parts, m)
	}
	if total, free, ok := diskSpace(dataVolume()); ok {
		parts = append(parts, fmt.Sprintf("disk %s, %s free", gb(total), gb(free)))
	}
	return strings.Join(parts, ", ")
}

// gb writes a byte count in decimal gigabytes, as Finder and Mole do.
func gb(n uint64) string { return fmt.Sprintf("%.0f GB", float64(n)/1e9) }

// isShellTool reports the tools whose command is a shell command line. The
// others (read_file, fetch_url, web_search…) take a path, an address or a
// query, and are written so that they do not read as commands.
func isShellTool(tool string) bool {
	return tool == "run_command" || tool == "disk_usage" || tool == "mac_change" || strings.HasPrefix(tool, "run_step:")
}

// Limits of what one call writes to the transcript. Past logCap lines only
// errors are written as they come, and the last logTail lines at the end:
// a brew upgrade that failed 13,000 times must not bury the run.
const (
	logCap       = 300
	logTail      = 40
	logErrors    = 30
	repeatsShown = 3
)

// callLog is what the transcript holds of one tool call while it runs.
type callLog struct {
	buffered bool     // part of a parallel batch: written in one block at the end
	lines    []string // the block, when buffered
	written  int
	omitted  int
	errs     int
	tail     []string
	shape    string // shape of the last line, to fold repeats
	run      int    // how many lines in a row had that shape
	hidden   int    // of those, how many were not written
}

func callKey(e Event) string { return e.Tool + "\x00" + e.Command }

// put adds a line to the call's block, or writes it.
func (a *Agent) put(cl *callLog, line string) {
	if cl != nil && cl.buffered {
		cl.lines = append(cl.lines, line)
		return
	}
	fmt.Fprintln(a.log, line)
}

var errorLine = regexp.MustCompile(`(?i)\b(error|fail(ed|ure)?|fatal|denied|refused|not permitted|no space left)\b`)

// outputLine applies the limits to one line of a call's output.
func (a *Agent) outputLine(cl *callLog, text string) {
	sh := lineShape(text)
	if sh == cl.shape {
		cl.run++
	} else {
		a.flushRepeats(cl)
		cl.shape, cl.run = sh, 1
	}
	if cl.run > repeatsShown {
		cl.hidden++
		return
	}
	line := "  " + text
	if cl.written < logCap {
		cl.written++
		a.put(cl, line)
		return
	}
	if errorLine.MatchString(text) && cl.errs < logErrors {
		cl.errs++
		a.put(cl, line)
		return
	}
	cl.omitted++
	cl.tail = append(cl.tail, line)
	if len(cl.tail) > logTail {
		cl.tail = cl.tail[1:]
	}
}

// flushRepeats says how many lines like the last one were folded.
func (a *Agent) flushRepeats(cl *callLog) {
	if cl.hidden > 0 {
		a.put(cl, fmt.Sprintf("  … %d more lines like the one above", cl.hidden))
	}
	cl.hidden, cl.run, cl.shape = 0, 0, ""
}

// endCall writes what is left of a call's output and its closing line.
func (a *Agent) endCall(cl *callLog, end string) {
	if cl != nil {
		a.flushRepeats(cl)
		if cl.omitted > len(cl.tail) {
			a.put(cl, fmt.Sprintf("  … %d lines not written to this transcript …", cl.omitted-len(cl.tail)))
		}
		for _, l := range cl.tail {
			a.put(cl, l)
		}
	}
	a.put(cl, end)
	if cl != nil && cl.buffered {
		for _, l := range cl.lines {
			fmt.Fprintln(a.log, l)
		}
	}
}

var (
	reDigits  = regexp.MustCompile(`[0-9]+`)
	reHexish  = regexp.MustCompile(`\b[0-9a-f]{12,}\b`)
	reQuoted  = regexp.MustCompile(`'[^']*'|"[^"]*"`)
	reNonWord = regexp.MustCompile(`[^\p{L}\p{N}# ]+`)
)

// lineShape is what two lines of the same kind have in common: the start of
// a path and the first words of the message after it, with numbers, hashes
// and quoted names taken out. 13,000 lines of
// "awscli/2.41.1/…/x.rst: Failed to create dir '…': No space left on device"
// share one shape; two different messages do not.
func lineShape(line string) string {
	s := strings.TrimSpace(line)
	if s == "" {
		return ""
	}
	s = reHexish.ReplaceAllString(s, "#")
	s = reQuoted.ReplaceAllString(s, "'…'")
	s = reDigits.ReplaceAllString(s, "#")
	head, msg, found := strings.Cut(s, ": ")
	if !found {
		if strings.Count(s, "/") >= 2 {
			parts := strings.SplitN(s, "/", 3)
			return parts[0] + "/" + parts[1] + "/…"
		}
		return s
	}
	if strings.Contains(head, "/") {
		parts := strings.SplitN(head, "/", 3)
		if len(parts) == 3 {
			head = parts[0] + "/" + parts[1] + "/…"
		}
	}
	words := strings.Fields(reNonWord.ReplaceAllString(msg, " "))
	if len(words) > 3 {
		words = words[:3]
	}
	return head + ": " + strings.Join(words, " ")
}

// collapseRepeats folds runs of lines of the same shape in the output a
// command hands the model: the first few stay, the rest is counted.
func collapseRepeats(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= repeatsShown {
		return text
	}
	var out []string
	shape, run, hidden := "", 0, 0
	flush := func() {
		if hidden > 0 {
			out = append(out, fmt.Sprintf("… %d more lines like the one above", hidden))
		}
		hidden = 0
	}
	for _, l := range lines {
		sh := lineShape(l)
		if sh != "" && sh == shape {
			run++
		} else {
			flush()
			shape, run = sh, 1
		}
		if run > repeatsShown {
			hidden++
			continue
		}
		out = append(out, l)
	}
	flush()
	return strings.Join(out, "\n")
}

func (a *Agent) writeLog(e Event) {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	if a.log == nil {
		return
	}
	if a.calls == nil {
		a.calls = map[string]*callLog{}
	}
	switch e.Kind {
	case EvAssistant:
		fmt.Fprintf(a.log, "\n%s\n", e.Text)
	case EvToolStart:
		cl := &callLog{buffered: a.batching > 0}
		a.calls[callKey(e)] = cl
		a.put(cl, "")
		if isShellTool(e.Tool) {
			a.put(cl, "$ "+e.Command)
		} else {
			a.put(cl, "» "+strings.TrimSpace(e.Tool+" "+strings.TrimPrefix(e.Command, e.Tool)))
		}
	case EvOutput:
		cl := a.calls[callKey(e)]
		if cl == nil {
			cl = &callLog{}
			a.calls[callKey(e)] = cl
		}
		a.outputLine(cl, e.Text)
	case EvToolEnd:
		key := callKey(e)
		cl := a.calls[key]
		delete(a.calls, key)
		a.endCall(cl, toolEndLine(e))
		a.tally(e)
	case EvConfirm:
		fmt.Fprintf(a.log, "\n? approval (%s): %s\n", e.Risk, e.Command)
	case EvChoice:
		kind := "one"
		if e.Multi {
			kind = "several"
		}
		fmt.Fprintf(a.log, "\n? choice (pick %s): %s\n", kind, e.Text)
		for _, c := range e.Choices {
			fmt.Fprintf(a.log, "  - %s: %s", c.ID, c.Label)
			if c.Risk != "" {
				fmt.Fprintf(a.log, " [%s]", c.Risk)
			}
			if c.Impact != "" {
				fmt.Fprintf(a.log, " (%s)", c.Impact)
			}
			fmt.Fprintln(a.log)
		}
	case EvDenied:
		fmt.Fprintf(a.log, "\n[%s DENIED] %s\n  reason: %s\n", e.Tool, e.Command, e.Reason)
	case EvPassword:
		fmt.Fprintf(a.log, "\n? sudo password requested (%s)\n", e.Text)
	case EvError:
		if e.Tool != "" {
			fmt.Fprintf(a.log, "\n[%s FAILED] %s\n", e.Tool, e.Text)
		} else {
			fmt.Fprintf(a.log, "\n! %s\n", e.Text)
		}
	case EvDone:
		// What ran, counted by omega: it is there even when the session
		// was cut before the model wrote its summary.
		if a.ok+len(a.failed) > 0 {
			fmt.Fprintf(a.log, "\n== tools: %d succeeded, %d failed", a.ok, len(a.failed))
			if len(a.failed) > 0 {
				fmt.Fprintf(a.log, ": %s", strings.Join(a.failed, ", "))
			}
			fmt.Fprintln(a.log)
		}
		end := "\n== end"
		if e.Text != "" {
			end += " (" + e.Text + ")"
		}
		fmt.Fprintf(a.log, "%s. tokens: %d in (%d cached), %d out, $%.4f%s\n", end, a.usage.Input(), a.usage.CacheRead, a.usage.OutputTokens, a.cost, a.costSplit())
	}
}

// tally counts a finished call for the closing line of the transcript.
func (a *Agent) tally(e Event) {
	if e.Tool == "remember" {
		return
	}
	if e.Failed || e.ExitCode != 0 {
		name := e.Tool
		if e.ExitCode != 0 {
			name += fmt.Sprintf(" (exit %d)", e.ExitCode)
		}
		a.failed = append(a.failed, name)
		return
	}
	a.ok++
}

// toolEndLine is the closing line of a call: its exit code and time, and
// why it failed when the tool says.
func toolEndLine(e Event) string {
	d := e.Duration.Round(time.Second)
	switch {
	case e.Tool == "remember":
		return "[remember] " + e.Command
	case e.Failed && e.ExitCode == 0:
		line := fmt.Sprintf("[%s FAILED %s]", e.Tool, d)
		if e.Reason != "" {
			line += " " + e.Reason
		}
		return line
	case e.ExitCode == -1:
		return fmt.Sprintf("[%s exit=-1 %s] (stopped: timed out or cancelled)", e.Tool, d)
	}
	return fmt.Sprintf("[%s exit=%d %s]", e.Tool, e.ExitCode, d)
}

// logResult writes the start of what a tool that is not a shell command
// returned: what the summary may later cite (the RAM, a version, a note)
// can then be checked against the transcript.
func (a *Agent) logResult(tool, out string) {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	if a.log == nil {
		return
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	fmt.Fprintf(a.log, "[%s returned %d lines]\n", tool, len(lines))
	for i, l := range lines {
		if i == resultLines {
			fmt.Fprintf(a.log, "  | … %d more lines\n", len(lines)-resultLines)
			break
		}
		fmt.Fprintf(a.log, "  | %s\n", l)
	}
}

// resultLines is how much of a non-shell tool's result goes in the
// transcript.
const resultLines = 15

// loggedResult are the tools whose result logResult writes. Shell tools
// stream their output already; propose_actions writes its own lines.
var loggedResult = map[string]bool{"system_info": true, "read_file": true, "fetch_url": true, "web_search": true,
	"dev_inventory": true, "mac_inventory": true, "run_history": true, "list_steps": true, "config_hints": true,
	"official_docs": true, "notes": true, "omega_doctor": true, "omega_settings": true, "mac_history": true,
	"model_ranking": true, "mac_policy": true, "pending": true}

// costSplit says what each model cost when the run used more than one.
func (a *Agent) costSplit() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.costBy) < 2 {
		return ""
	}
	var names []string
	for m := range a.costBy {
		names = append(names, m)
	}
	sort.Strings(names)
	var parts []string
	for _, m := range names {
		parts = append(parts, fmt.Sprintf("%s $%.4f", m, a.costBy[m]))
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func (a *Agent) closeLog() {
	if a.log != nil {
		a.log.Close()
		a.log = nil
	}
}
