package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lcajigasm/omega/internal/i18n"
)

// The checks of the command line that need no model: the desktop app offers
// them as tools. Each one is a command of omega, run as it is and with what
// it prints returned, so both show the same thing.

// guiTool is one command the desktop app can run.
type guiTool struct {
	id    string
	group string   // net, security, disk, dev, system
	args  []string // the command line after `omega`
	// arg says what the optional argument is: "port", "dir", "days" or "file".
	arg string
	// def is the argument used when none is given; "~" stands for the home.
	def string
}

// guiTools are the tools in the order the app lists them. Only commands
// that look or write a file the user names are here: the ones that delete
// (git clean, uninstall, privacy --clean) ask on a terminal and stay there.
var guiTools = []guiTool{
	{id: "net-ping", group: "net", args: []string{"net", "ping"}},
	{id: "net-ports", group: "net", args: []string{"net", "ports"}, arg: "port"},
	{id: "net-audit", group: "net", args: []string{"net", "audit"}},
	{id: "audit-ssh", group: "security", args: []string{"audit", "ssh"}},
	{id: "audit-secrets", group: "security", args: []string{"audit", "secrets"}},
	{id: "privacy", group: "security", args: []string{"privacy"}},
	{id: "disk-large", group: "disk", args: []string{"disk", "large"}, arg: "dir", def: "~"},
	{id: "disk-duplicates", group: "disk", args: []string{"disk", "duplicates"}, arg: "dir", def: "~/Downloads"},
	{id: "disk-artifacts", group: "disk", args: []string{"disk", "artifacts"}, arg: "days"},
	{id: "git-status", group: "dev", args: []string{"git", "status"}},
	{id: "schedule-status", group: "system", args: []string{"schedule", "status"}},
	{id: "rollback", group: "system", args: []string{"rollback"}},
	{id: "export", group: "system", args: []string{"export"}, arg: "file", def: "~/omega-blueprint.json"},
}

// ToolInfo is a tool as the desktop app shows it.
type ToolInfo struct {
	ID      string `json:"id"`
	Group   string `json:"group"` // heading, translated
	Title   string `json:"title"`
	Detail  string `json:"detail"`
	Command string `json:"command"` // the same thing on a terminal
	Arg     string `json:"arg"`     // label of the optional argument; "" when it takes none
	Default string `json:"default"` // what the argument is when left empty
}

// ToolList lists the tools, grouped.
func ToolList() []ToolInfo {
	out := []ToolInfo{}
	for _, t := range guiTools {
		in := ToolInfo{ID: t.id, Group: i18n.T("gui.tools.g." + t.group), Title: i18n.T("gui.tool." + t.id),
			Detail: i18n.T("gui.tool." + t.id + ".detail"), Command: "omega " + strings.Join(t.args, " "), Default: t.def}
		if t.arg != "" {
			in.Arg = i18n.T("gui.tools.arg." + t.arg)
		}
		out = append(out, in)
	}
	return out
}

// homePath expands a leading ~ to the home directory.
func homePath(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// RunTool runs a tool and returns what the command printed. arg is its
// optional argument; an argument that looks like a flag is refused, so the
// app can only run the commands listed here.
func RunTool(id, arg string) (string, error) {
	for _, t := range guiTools {
		if t.id != id {
			continue
		}
		args := append([]string(nil), t.args...)
		if arg = strings.TrimSpace(arg); arg == "" {
			arg = t.def
		}
		if t.arg != "" && arg != "" {
			if strings.HasPrefix(arg, "-") {
				return "", fmt.Errorf("%s", i18n.T("gui.tools.bad_arg", arg))
			}
			if t.arg == "dir" || t.arg == "file" {
				arg = homePath(arg)
			}
			args = append(args, arg)
		}
		var f runFlags
		root := newRoot(&f)
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(args)
		err := root.Execute()
		return strings.TrimRight(out.String(), "\n"), err
	}
	return "", fmt.Errorf("%s", i18n.T("gui.tools.unknown", id))
}
