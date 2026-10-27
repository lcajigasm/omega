package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lcajigasm/omega/internal/config"
)

func (a *Agent) openLog(provider string) {
	dir := filepath.Join(config.StateDir(), "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	f, err := os.Create(filepath.Join(dir, time.Now().Format("2006-01-02_150405")+".log"))
	if err != nil {
		return
	}
	a.log = f
	fmt.Fprintf(f, "omega run %s — %s / %s\n\n", time.Now().Format(time.RFC3339), provider, a.opts.Model)
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
	case EvError:
		fmt.Fprintf(a.log, "\n! %s\n", e.Text)
	case EvDone:
		fmt.Fprintf(a.log, "\n== end. tokens: %d in, %d out\n", a.usage.InputTokens, a.usage.OutputTokens)
	}
}

func (a *Agent) closeLog() {
	if a.log != nil {
		a.log.Close()
	}
}
