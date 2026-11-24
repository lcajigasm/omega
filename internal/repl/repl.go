// Package repl provides an interactive maintenance shell / REPL for running
// repetitive diagnostics, health inspections, and updates without restarting the process.
package repl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/lcajigasm/omega/internal/i18n"
)

// RunnerFunc is the signature for executing an Omega command or task.
type RunnerFunc func(ctx context.Context, args []string) error

// REPL manages the interactive console loop.
type REPL struct {
	in       io.Reader
	out      io.Writer
	commands map[string]RunnerFunc
	version  string
}

// New creates a new REPL instance.
func New(in io.Reader, out io.Writer, version string) *REPL {
	return &REPL{
		in:       in,
		out:      out,
		commands: make(map[string]RunnerFunc),
		version:  version,
	}
}

// Register adds a command handler to the REPL.
func (r *REPL) Register(name string, fn RunnerFunc) {
	r.commands[name] = fn
}

// Run starts the interactive loop.
func (r *REPL) Run(ctx context.Context) error {
	fmt.Fprintf(r.out, "%s\n\n", i18n.T("repl.welcome", r.version))

	scanner := bufio.NewScanner(r.in)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		fmt.Fprint(r.out, "omega> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		cmd := fields[0]
		args := fields[1:]

		if cmd == "exit" || cmd == "quit" || cmd == "q" {
			fmt.Fprintln(r.out, i18n.T("repl.bye"))
			break
		}

		if cmd == "help" || cmd == "?" {
			r.printHelp()
			continue
		}

		fn, ok := r.commands[cmd]
		if !ok {
			fmt.Fprintln(r.out, i18n.T("repl.unknown", cmd))
			continue
		}

		if err := fn(ctx, args); err != nil {
			fmt.Fprintln(r.out, failure(err))
		}
		fmt.Fprintln(r.out)
	}

	return scanner.Err()
}

// failure is the line shown for a command that failed. A run that already
// showed what went wrong returns an error with no text and an exit status:
// "Error: " followed by nothing would say less than the status does.
func failure(err error) string {
	if msg := strings.TrimSpace(err.Error()); msg != "" {
		return i18n.T("repl.error", msg)
	}
	var code interface{ ExitCode() int }
	if errors.As(err, &code) {
		return i18n.T("repl.failed_exit", code.ExitCode())
	}
	return i18n.T("repl.failed")
}

func (r *REPL) printHelp() {
	fmt.Fprintln(r.out, i18n.T("repl.help_title"))
	for _, c := range []struct{ name, key string }{
		{"quick", "repl.help_quick"}, {"update", "repl.help_update"}, {"clean", "repl.help_clean"},
		{"analyze", "repl.help_analyze"}, {"health", "repl.help_health"}, {"doctor", "repl.help_doctor"},
		{"status", "repl.help_status"}, {"dev", "repl.help_dev"}, {"net", "repl.help_net"},
		{"audit", "repl.help_audit"}, {"privacy", "repl.help_privacy"}, {"steps", "repl.help_steps"},
		{"help, ?", "repl.help_help"}, {"exit, quit", "repl.help_exit"},
	} {
		fmt.Fprintf(r.out, "  %-10s - %s\n", c.name, i18n.T(c.key))
	}
}
