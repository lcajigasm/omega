package agent

import (
	"fmt"
	"os"
	"path/filepath"
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	path := filepath.Join(dir, time.Now().Format("2006-01-02_150405")+".log")
	f, err := os.Create(path)
	if err != nil {
		return
	}
	a.log = f
	fmt.Fprintf(f, "omega run %s — %s / %s\n\n", time.Now().Format(time.RFC3339), provider, a.opts.Model)
	rotateRunLogs(dir, path, MaxRunLogs)
}

func (a *Agent) writeLog(e Event) {
	if a.log == nil {
		return
	}
	switch e.Kind {
	case EvAssistant:
		fmt.Fprintf(a.log, "\n%s\n", e.Text)
	case EvToolStart:
		fmt.Fprintf(a.log, "\n$ %s\n", e.Command)
	case EvOutput:
		fmt.Fprintf(a.log, "  %s\n", e.Text)
	case EvToolEnd:
		fmt.Fprintf(a.log, "[%s exit=%d %s]\n", e.Tool, e.ExitCode, e.Duration.Round(time.Second))
	case EvConfirm:
		fmt.Fprintf(a.log, "\n? approval (%s): %s\n", e.Risk, e.Command)
	case EvPassword:
		fmt.Fprintf(a.log, "\n? sudo password requested (%s)\n", e.Text)
	case EvError:
		fmt.Fprintf(a.log, "\n! %s\n", e.Text)
	case EvDone:
		fmt.Fprintf(a.log, "\n== end. tokens: %d in (%d cached), %d out, $%.4f\n", a.usage.Input(), a.usage.CacheRead, a.usage.OutputTokens, a.cost)
	}
}

func (a *Agent) closeLog() {
	if a.log != nil {
		a.log.Close()
		a.log = nil
	}
}
