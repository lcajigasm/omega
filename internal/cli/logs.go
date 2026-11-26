package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/ui"
)

// logFilter narrows `omega logs`: how many, which task, only the failed.
type logFilter struct {
	last   int
	task   string
	failed bool
}

func logsCmd() *cobra.Command {
	var lf logFilter
	var asJSON bool
	c := &cobra.Command{
		Use:   "logs",
		Short: i18n.T("cmd.logs"),
		Long:  i18n.T("cmd.logs_long"),
		Args:  cobra.NoArgs,
		Example: "  omega logs\n  omega logs -n 5 --failed\n  omega logs --task clean --json\n" +
			"  omega logs view 2\n  omega logs view --tools",
		Annotations: map[string]string{"see_also": "omega history, omega pending, omega notes"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if asJSON {
				runs, err := logEntries(lf)
				if err != nil {
					return err
				}
				return writeJSON(cmd.OutOrStdout(), runs)
			}
			return writeLogList(cmd.OutOrStdout(), lf)
		},
	}
	c.Flags().IntVarP(&lf.last, "last", "n", 0, i18n.T("flag.logs_last"))
	c.Flags().StringVar(&lf.task, "task", "", i18n.T("flag.logs_task"))
	c.Flags().BoolVar(&lf.failed, "failed", false, i18n.T("flag.logs_failed"))
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.json"))
	var tools bool
	view := &cobra.Command{
		Use:               "view [n|file]",
		Short:             i18n.T("cmd.logs_view"),
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeLogs,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return writeLogView(cmd.OutOrStdout(), name, tools)
		},
	}
	view.Flags().BoolVar(&tools, "tools", false, i18n.T("flag.logs_tools"))
	c.AddCommand(view)
	return c
}

// logEntry is one run as `omega logs --json` and `omega history` give it.
type logEntry struct {
	N       int       `json:"n"`
	File    string    `json:"file"`
	Path    string    `json:"path"`
	When    time.Time `json:"when"`
	Task    string    `json:"task,omitempty"`
	Model   string    `json:"model,omitempty"`
	CostUSD float64   `json:"cost_usd"`
	Failed  int       `json:"failed_tools"`
	Ended   bool      `json:"ended"`
	Bytes   int64     `json:"bytes"`
}

// logEntries reads the runs, newest first, and keeps those the filter asks
// for. N is the number `omega logs view` takes, counted before filtering.
func logEntries(lf logFilter) ([]logEntry, error) {
	logs, err := agent.ListRunLogs()
	if err != nil {
		return nil, err
	}
	out := []logEntry{}
	for i, l := range logs {
		s := summarizeLog(l.Path)
		when := s.when
		if when.IsZero() {
			// An old or cut log has no header: its name is its start.
			if t, err := time.ParseInLocation("2006-01-02_150405", strings.TrimSuffix(l.Name, ".log"), time.Local); err == nil {
				when = t
			} else {
				when = l.ModTime
			}
		}
		e := logEntry{N: i + 1, File: l.Name, Path: l.Path, When: when, Task: s.task, Model: s.model,
			CostUSD: s.cost, Failed: s.failed, Ended: s.ended, Bytes: l.Size}
		if lf.task != "" && !strings.EqualFold(e.Task, lf.task) {
			continue
		}
		if lf.failed && e.Ended && e.Failed == 0 {
			continue
		}
		out = append(out, e)
		if lf.last > 0 && len(out) == lf.last {
			break
		}
	}
	return out, nil
}

func writeLogList(w io.Writer, lf logFilter) error {
	all, err := agent.ListRunLogs()
	if err != nil {
		return err
	}
	if len(all) == 0 {
		writeEmpty(w, "omega logs", "", i18n.T("logs.empty"), i18n.T("logs.empty_hint"))
		return nil
	}
	runs, err := logEntries(lf)
	if err != nil {
		return err
	}
	r := ui.NewReport(w)
	r.Title("omega logs", i18n.N("logs.header2", len(all), len(all), agent.MaxRunLogs))
	r.Blank()
	if len(runs) == 0 {
		r.Line(r.Muted(i18n.T("logs.none_match")))
		return nil
	}
	var rows [][]string
	var marks []ui.Mark
	var total float64
	for _, e := range runs {
		mark := ui.MarkOK
		state := ""
		switch {
		case !e.Ended:
			mark, state = ui.MarkWarn, r.Warn(i18n.T("logs.unfinished"))
		case e.Failed > 0:
			mark, state = ui.MarkErr, r.Err(i18n.T("logs.failed_tools", e.Failed))
		}
		cost := ""
		if e.Ended {
			cost = ui.FormatCost(e.CostUSD)
			total += e.CostUSD
		}
		task := e.Task
		if task == "" {
			task = "—"
		}
		rows = append(rows, []string{fmt.Sprint(e.N), e.When.Format("2006-01-02 15:04"), task, r.Muted(e.Model), cost, state})
		marks = append(marks, mark)
	}
	r.Table([]ui.Column{
		{Title: "#", Right: true}, {Title: i18n.T("logs.col_when")}, {Title: i18n.T("logs.col_task")},
		{Title: i18n.T("logs.col_model")}, {Title: i18n.T("logs.col_cost"), Right: true}, {Title: ""},
	}, rows, marks)
	r.Blank()
	r.Line(r.Muted(i18n.T("logs.total", ui.FormatCost(total), tildePath(agent.RunsDir()))))
	r.Hint(i18n.T("logs.hint"))
	return nil
}

// logSummary is what the list shows of one transcript, read from its
// header and its closing line.
type logSummary struct {
	when   time.Time
	model  string
	task   string
	cost   float64
	failed int
	ended  bool
}

var (
	logHead   = regexp.MustCompile(`^omega run (\S+) — (.+)$`)
	logTask   = regexp.MustCompile(`\(task: ([^)]+)\)\s*$`)
	logEnd    = regexp.MustCompile(`(?m)^== end.*\$([0-9.]+)`)
	logFailed = regexp.MustCompile(`(?m)^== tools: \d+ succeeded, (\d+) failed`)
)

// summarizeLog reads the first lines and the last few kilobytes of a
// transcript; it never reads a long one whole.
func summarizeLog(path string) logSummary {
	var s logSummary
	f, err := os.Open(path)
	if err != nil {
		return s
	}
	defer f.Close()
	head := make([]byte, 2048)
	n, _ := io.ReadFull(f, head)
	for i, line := range strings.SplitN(string(head[:n]), "\n", 5) {
		if i == 0 {
			if m := logHead.FindStringSubmatch(line); m != nil {
				s.when, _ = time.Parse(time.RFC3339, m[1])
				s.model = strings.TrimSpace(m[2])
				if _, model, ok := strings.Cut(s.model, " / "); ok {
					s.model = model
				}
			}
		}
		if strings.HasPrefix(line, "command: ") {
			if m := logTask.FindStringSubmatch(line); m != nil {
				s.task = m[1]
			}
		}
	}
	const tail = 4096
	if info, err := f.Stat(); err == nil {
		off := max(0, info.Size()-tail)
		buf := make([]byte, info.Size()-off)
		if _, err := f.ReadAt(buf, off); err == nil || errors.Is(err, io.EOF) {
			if m := logEnd.FindAllStringSubmatch(string(buf), -1); m != nil {
				s.ended = true
				s.cost, _ = strconv.ParseFloat(m[len(m)-1][1], 64)
			}
			if m := logFailed.FindAllStringSubmatch(string(buf), -1); m != nil {
				s.failed, _ = strconv.Atoi(m[len(m)-1][1])
			}
		}
	}
	return s
}

func writeLogView(w io.Writer, name string, tools bool) error {
	_, b, err := agent.ReadRunLog(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if name == "" {
				return fmt.Errorf("%s", i18n.T("logs.empty"))
			}
			return fmt.Errorf("%s", i18n.T("logs.missing", name))
		}
		return err
	}
	if tools {
		b = []byte(toolLines(string(b)))
	}
	fmt.Fprint(w, string(b))
	return nil
}

// toolLines keeps the tool calls of a transcript and what they returned:
// the call ("» tool args"), the command ("$ …"), the status lines
// ("[tool exit=0 1s]", "[tool returned N lines]"), their indented output and
// the closing tally. The model's prose and the prompts are left out.
func toolLines(text string) string {
	var b strings.Builder
	for _, l := range strings.SplitAfter(text, "\n") {
		switch {
		case strings.HasPrefix(l, "» "), strings.HasPrefix(l, "$ "), strings.HasPrefix(l, "["),
			strings.HasPrefix(l, "  "), strings.HasPrefix(l, "== tools"):
			if strings.HasPrefix(l, "» ") && b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(l)
		}
	}
	return b.String()
}
