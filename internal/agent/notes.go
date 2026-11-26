package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/safety"
)

// The notes omega keeps between runs: remember adds one, the notes tool
// lists and forgets them, and every run reads the newest ones with its
// instructions.

// maxNote is the longest note remember keeps, in characters: a fact about
// this machine, not a page of instructions.
const maxNote = 400

// maxPromptNotes is how much of the notes, newest first, goes into the
// instructions of a run. Machines collected a hundred notes, many of them
// the same one again, and every turn paid for all of them.
const maxPromptNotes = 4 * 1024

// cleanNote makes a note one plain line: control characters and the marks
// that reorder or hide text are dropped.
func cleanNote(note string) string {
	note = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == ' ' || r == ' ' {
			return -1
		}
		return r
	}, note)
	return strings.Join(strings.Fields(note), " ")
}

// leadingDate is a date at the start of a note, with what joins it to the
// text: "2026-10-27: ", "2026-10-27 — ", "2026-10-27 (2): ".
var leadingDate = regexp.MustCompile(`^\s*\d{4}-\d{2}-\d{2}(\s*\(\d+\))?\s*[:—–-]?\s*`)

// noteKey is what two notes that say the same thing have in common: the
// text without the list mark, the date, the case, the spacing and the
// final punctuation.
func noteKey(line string) string {
	s := strings.TrimSpace(line)
	s = strings.TrimPrefix(s, "- ")
	s = leadingDate.ReplaceAllString(s, "")
	s = strings.ToLower(strings.Join(strings.Fields(s), " "))
	return strings.TrimRight(s, ".;:!, ")
}

// readNotes returns the saved notes, oldest first.
func readNotes() []string {
	data, _ := os.ReadFile(config.NotesPath())
	var out []string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// promptNotes is the part of the notes a run is given: the newest first,
// up to max bytes, and a line saying how many older ones were left out.
func promptNotes(notes string, max int) string {
	var lines []string
	for _, l := range strings.Split(notes, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	var b strings.Builder
	kept := 0
	for i := len(lines) - 1; i >= 0; i-- {
		if b.Len()+len(lines[i])+1 > max && kept > 0 {
			break
		}
		b.WriteString(lines[i] + "\n")
		kept++
	}
	if left := len(lines) - kept; left > 0 {
		fmt.Fprintf(&b, "(%d older notes are not shown here; the notes tool lists them all)\n", left)
	}
	return b.String()
}

// readOnlyDenial refuses a write to what outlives the run in a run that
// must not change anything: --dry-run, or read-only mode.
func (a *Agent) readOnlyDenial(tool, command, what string) (string, bool) {
	if a.Mode() != ModeReadOnly {
		return "", false
	}
	why := "Not saved: this run is read-only (--dry-run or read-only mode), so " + what + " was not written. " +
		"Say in your summary what you would have recorded."
	a.emit(Event{Kind: EvDenied, Tool: tool, Command: command, Reason: why})
	return why, true
}

// toolRemember saves a note that every later run reads with its
// instructions. Once a web page or a search result is in the conversation
// the note may be the page's words, not something omega learned, so the user
// sees it and says yes first; where nobody can, it is not saved.
func toolRemember(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	note := cleanNote(getString(in, "note"))
	if note == "" {
		return "", fmt.Errorf("empty note")
	}
	if n := utf8.RuneCountInString(note); n > maxNote {
		return "", fmt.Errorf("not saved: a note is one short line (at most %d characters); this one has %d. Write it again, shorter", maxNote, n)
	}
	// omega dates the note itself; a date the model put in front as well
	// would be read twice ("2026-10-27: 2026-10-27 full run").
	note = strings.TrimSpace(leadingDate.ReplaceAllString(note, ""))
	if why, denied := a.readOnlyDenial("remember", note, "the note"); denied {
		return why, nil
	}
	key := noteKey(note)
	for _, l := range readNotes() {
		if noteKey(l) == key {
			return "Not saved again: an equal note is already kept (" + strings.TrimPrefix(strings.TrimSpace(l), "- ") + ").", nil
		}
	}
	if a.isTainted() {
		if a.opts.Unattended {
			why := "Not saved: this session has read a web page, search results or other text from outside, and a note written after that needs " +
				"the user's yes, which nobody can give in this run. In your summary say the note was not saved because the session read the web, and quote it."
			a.emit(Event{Kind: EvDenied, Tool: "remember", Command: note, Reason: why})
			return "", explained(errors.New(why))
		}
		// Asked in every mode: the note outlives this session.
		if ok, why := a.authorize(ctx, "remember", note, i18n.T("agent.remember_reason"), safety.Dangerous, true); !ok {
			return "", explained(errors.New(why))
		}
	}
	if err := os.MkdirAll(config.Dir(), 0o755); err != nil {
		return "", err
	}
	f, err := os.OpenFile(config.NotesPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := fmt.Fprintf(f, "- %s: %s\n", time.Now().Format("2006-01-02"), note); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	a.forget()
	a.emit(Event{Kind: EvToolEnd, Tool: "remember", Command: i18n.T("agent.note_saved", note)})
	return "saved", nil
}

func toolNotes(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	lines := readNotes()
	if getString(in, "action") != "forget" {
		if len(lines) == 0 {
			return "There are no notes yet.", nil
		}
		return strings.Join(lines, "\n"), nil
	}
	match := strings.ToLower(getString(in, "match"))
	if match == "" {
		return "", fmt.Errorf("forget needs match: a text the notes to delete contain")
	}
	var keep, drop []string
	for _, l := range lines {
		if strings.Contains(strings.ToLower(l), match) {
			drop = append(drop, l)
		} else {
			keep = append(keep, l)
		}
	}
	if len(drop) == 0 {
		return "No note contains that text; nothing was deleted.", nil
	}
	if why, denied := a.readOnlyDenial("notes", "forget "+match, "the change to the notes"); denied {
		return why, nil
	}
	var preview []string
	for _, l := range drop {
		preview = append(preview, "- "+strings.TrimPrefix(l, "- "))
	}
	reason := i18n.T("agent.notes_forget") + "\n\n" + strings.Join(preview, "\n")
	// The notes outlive the run: a person says yes every time, and a run
	// nobody watches leaves them as they are.
	if ok, why := a.authorize(ctx, "notes", config.NotesPath(), reason, safety.Dangerous, true); !ok {
		return "", explained(errors.New(why))
	}
	body := ""
	if len(keep) > 0 {
		body = strings.Join(keep, "\n") + "\n"
	}
	if err := writeAtomic(config.NotesPath(), []byte(body), 0o600); err != nil {
		return "", err
	}
	a.forget()
	a.emit(Event{Kind: EvToolEnd, Tool: "notes", Command: i18n.N("agent.notes_forgot", len(drop))})
	return fmt.Sprintf("Deleted %d note(s).", len(drop)), nil
}
