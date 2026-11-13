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
	fmt.Fprintf(r.out, "Omega %s interactive shell. Type 'help' for commands or 'exit' to quit.\n\n", r.version)

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
			fmt.Fprintln(r.out, "Goodbye!")
			break
		}

		if cmd == "help" || cmd == "?" {
			r.printHelp()
			continue
		}

		fn, ok := r.commands[cmd]
		if !ok {
			fmt.Fprintf(r.out, "Unknown command '%s'. Type 'help' for available commands.\n", cmd)
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
		return "Error: " + msg
	}
	var code interface{ ExitCode() int }
	if errors.As(err, &code) {
		return fmt.Sprintf("failed (exit %d)", code.ExitCode())
	}
	return "failed"
}

func (r *REPL) printHelp() {
	fmt.Fprintln(r.out, "Available commands:")
	fmt.Fprintln(r.out, "  quick      - Run one-click safe maintenance")
	fmt.Fprintln(r.out, "  update     - Update software and package managers")
	fmt.Fprintln(r.out, "  clean      - Clean system caches and trash")
	fmt.Fprintln(r.out, "  analyze    - System report without changes")
	fmt.Fprintln(r.out, "  health     - Check disk, battery, RAM and logs")
	fmt.Fprintln(r.out, "  doctor     - Check Omega configuration and providers")
	fmt.Fprintln(r.out, "  status     - The same as doctor")
	fmt.Fprintln(r.out, "  dev        - Developer setup status and tools")
	fmt.Fprintln(r.out, "  net        - Network diagnostics and DNS flush")
	fmt.Fprintln(r.out, "  audit      - Security and SSH key audit")
	fmt.Fprintln(r.out, "  privacy    - Review telemetry and browser caches")
	fmt.Fprintln(r.out, "  steps      - Catalogue of native maintenance steps")
	fmt.Fprintln(r.out, "  help, ?    - Show this help message")
	fmt.Fprintln(r.out, "  exit, quit - Exit the shell")
}
