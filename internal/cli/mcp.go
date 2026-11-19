package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/assist"
	"github.com/lcajigasm/omega/internal/i18n"
)

// omega mcp is a Model Context Protocol server on stdin and stdout, so AI
// agents (Claude Code, Claude Desktop, Codex, Cursor, Gemini CLI, VS Code)
// can use omega instead of improvising commands. Each tool runs an omega
// command, the same one a person would type, with no terminal: nothing it
// starts can wait for an answer. What only reads is marked read-only; what
// writes a setting keeps a copy of the file first; what removes files is
// marked destructive and only says what it would do unless asked again.
// The model never gets a way to run an arbitrary command or omega task.

const mcpProtocol = "2025-06-18"

func mcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: i18n.T("cmd.mcp"),
		Long: "Speaks the Model Context Protocol on standard input and output. Register it in an agent:\n\n" +
			"  claude mcp add --scope user omega -- omega mcp\n" +
			"  codex mcp add omega -- omega mcp\n\n" +
			"or let omega do it: omega assist claude-code, claude-desktop, codex, cursor, gemini-cli or vscode.\n\n" +
			"Tools: " + strings.Join(mcpToolNames(), ", ") + ".\n" +
			"Read-only tools are marked so; settings keep a copy of the file they change; nothing is deleted\n" +
			"unless the agent asks for it explicitly, and the agent's client asks you first.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return serveMCP(cmd.Context(), os.Stdin, cmd.OutOrStdout())
		},
	}
}

type mcpTool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations"`
	call        func(ctx context.Context, args map[string]any) (string, error)
}

func schema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func readOnly(title string) map[string]any {
	return map[string]any{"title": title, "readOnlyHint": true, "openWorldHint": false}
}

func writes(title string, destructive bool) map[string]any {
	return map[string]any{"title": title, "readOnlyHint": false, "destructiveHint": destructive, "idempotentHint": !destructive, "openWorldHint": false}
}

func argString(args map[string]any, key string) string {
	switch v := args[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

// safeWord keeps arguments that become command-line words plain: no flags,
// no shell, nothing omega would read as an option.
var safeWord = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._,:/~ -]*$`)

func word(args map[string]any, key string, required bool) (string, error) {
	v := argString(args, key)
	if v == "" {
		if required {
			return "", fmt.Errorf("%s is required", key)
		}
		return "", nil
	}
	if !safeWord.MatchString(v) || len(v) > 300 {
		return "", fmt.Errorf("%s: %q is not a plain value", key, v)
	}
	return v, nil
}

func mcpTools() []mcpTool {
	return []mcpTool{
		{
			Name: "omega_disk_limits", Title: "Disk limits",
			Description: "What each AI app and developer tool (Claude Code, Claude Desktop, ChatGPT/Codex, Grok, Docker, Cursor, Homebrew, Xcode...) takes on disk, how it is limited now, the recommended limit and what can be freed. JSON. Use the check ids with omega_set_limit and omega_apply_check.",
			InputSchema: schema(map[string]any{}), Annotations: readOnly("Disk limits"),
			call: func(ctx context.Context, _ map[string]any) (string, error) {
				return runOmega(ctx, "disk", "limits", "--json")
			},
		},
		{
			Name: "omega_assistants", Title: "Configuration assistants",
			Description: "The configuration checks omega has for the AI apps and developer tools on this machine. Without a tool: every tool, installed or not, with how many checks are pending. With a tool (claude-code, claude-desktop, codex, grok, docker, cursor, vscode, gemini-cli, homebrew, xcode, ollama): its checks, current and recommended values. JSON.",
			InputSchema: schema(map[string]any{"tool": str("Tool id, optional")}), Annotations: readOnly("Configuration assistants"),
			call: func(ctx context.Context, args map[string]any) (string, error) {
				t, err := word(args, "tool", false)
				if err != nil {
					return "", err
				}
				if t == "" {
					return runOmega(ctx, "assist", "--json")
				}
				return runOmega(ctx, "assist", t, "--json")
			},
		},
		{
			Name: "omega_set_limit", Title: "Set a disk limit",
			Description: "Sets one disk limit by check id: a setting written in the tool's own configuration file (a copy of the file is kept first), or a budget omega keeps for a folder the tool never trims. Values: days (14), sizes (10GB, 20MB), 'off' to remove a budget. Ask the user before changing their configuration.",
			InputSchema: schema(map[string]any{"check": str("Check id, from omega_disk_limits"), "value": str("The limit")}, "check", "value"),
			Annotations: writes("Set a disk limit", false),
			call: func(ctx context.Context, args map[string]any) (string, error) {
				id, err := word(args, "check", true)
				if err != nil {
					return "", err
				}
				v, err := word(args, "value", true)
				if err != nil {
					return "", err
				}
				return runOmega(ctx, "disk", "limits", "set", id, v)
			},
		},
		{
			Name: "omega_apply_check", Title: "Apply a configuration check",
			Description: "Applies one setting or action of a configuration assistant by check id, with its recommended value or the one given: a limit, deny rules that keep the agent out of .env files and keys, registering omega's MCP server in another agent. A copy of each file is kept first. Cleanups that delete files are not done here: use omega_free_space. Ask the user first.",
			InputSchema: schema(map[string]any{"check": str("Check id, from omega_assistants"), "value": str("Optional value; the recommended one when left out")}, "check"),
			Annotations: writes("Apply a configuration check", false),
			call: func(ctx context.Context, args map[string]any) (string, error) {
				id, err := word(args, "check", true)
				if err != nil {
					return "", err
				}
				v, err := word(args, "value", false)
				if err != nil {
					return "", err
				}
				c, ok := assistScan().Check(id)
				if !ok {
					return "", fmt.Errorf("no check %q on this machine", id)
				}
				if c.Kind == assist.KindCleanup || c.Kind == assist.KindInfo || v == "now" {
					return "", fmt.Errorf("%s removes files or is measured only: use omega_free_space", id)
				}
				a := []string{"assist", "apply", id}
				if v != "" {
					a = append(a, v)
				}
				return runOmega(ctx, append(a, "--yes")...)
			},
		},
		{
			Name: "omega_free_space", Title: "Free space within the limits",
			Description: "Removes the old versions updaters leave behind (Claude Code, Cursor's agent...) and empties the folders that are over the budget the user set (Claude Desktop's VM image, app caches, DerivedData), never while their app is open. With dry_run true (the default) it only lists what it would remove; call it again with dry_run false once the user agrees.",
			InputSchema: schema(map[string]any{"dry_run": map[string]any{"type": "boolean", "description": "Only list what would be freed (default true)"}}),
			Annotations: writes("Free space within the limits", true),
			call: func(ctx context.Context, args map[string]any) (string, error) {
				if v, ok := args["dry_run"].(bool); ok && !v {
					return runOmega(ctx, "disk", "limits", "enforce", "--yes")
				}
				return runOmega(ctx, "disk", "limits", "enforce", "--dry-run")
			},
		},
		{
			Name: "omega_doctor", Title: "omega doctor",
			Description: "Checks omega's own setup, the AI provider key and the tools its maintenance steps use. Costs nothing and calls no model.",
			InputSchema: schema(map[string]any{}), Annotations: readOnly("omega doctor"),
			call: func(ctx context.Context, _ map[string]any) (string, error) { return runOmega(ctx, "doctor") },
		},
		{
			Name: "omega_analyze", Title: "Analyze the system",
			Description: "Runs omega's read-only analysis without a model: disk, memory, outdated packages, security settings, services. Returns one JSON report. Takes a minute or two.",
			InputSchema: schema(map[string]any{}), Annotations: readOnly("Analyze the system"),
			call: func(ctx context.Context, _ map[string]any) (string, error) { return runOmega(ctx, "analyze", "--json") },
		},
		{
			Name: "omega_agents", Title: "AI agents' leftovers",
			Description: "What closed AI coding agent sessions left running (dev servers, watchers, browsers) and the sessions still open, with ports, memory and age. JSON. Ending them is left to the user: omega agents kill.",
			InputSchema: schema(map[string]any{}), Annotations: readOnly("AI agents' leftovers"),
			call: func(ctx context.Context, _ map[string]any) (string, error) { return runOmega(ctx, "agents", "--json") },
		},
		{
			Name: "omega_large_files", Title: "Large files",
			Description: "Lists the 25 largest files above a size in a folder (the home folder by default).",
			InputSchema: schema(map[string]any{"dir": str("Folder, default the home folder"), "min_mb": map[string]any{"type": "integer", "description": "Smallest size in MB, default 100"}}),
			Annotations: readOnly("Large files"),
			call: func(ctx context.Context, args map[string]any) (string, error) {
				dir, err := word(args, "dir", false)
				if err != nil {
					return "", err
				}
				if dir == "" {
					dir, _ = os.UserHomeDir()
				}
				min := argString(args, "min_mb")
				if _, err := strconv.Atoi(min); err != nil {
					min = "100"
				}
				return runOmega(ctx, "disk", "large", expandHome(dir), min)
			},
		},
		{
			Name: "omega_logs", Title: "Run logs",
			Description: "Without n: the list of omega's run logs. With n (1 is the newest): that run's transcript.",
			InputSchema: schema(map[string]any{"n": map[string]any{"type": "integer", "description": "Which log, 1 is the newest"}}),
			Annotations: readOnly("Run logs"),
			call: func(ctx context.Context, args map[string]any) (string, error) {
				n := argString(args, "n")
				if _, err := strconv.Atoi(n); err != nil {
					return runOmega(ctx, "logs")
				}
				return runOmega(ctx, "logs", "view", n)
			},
		},
		{
			Name: "omega_docs", Title: "omega's manual",
			Description: "Searches omega's manual and returns the sections that answer a question (how to limit Docker's disk, what --yes does, how to schedule runs...). Use it before guessing an omega command.",
			InputSchema: schema(map[string]any{"query": str("Words or a question")}, "query"),
			Annotations: readOnly("omega's manual"),
			call: func(_ context.Context, args map[string]any) (string, error) {
				q := argString(args, "query")
				if q == "" {
					return "", fmt.Errorf("query is required")
				}
				return manualAnswerContext(q, 4), nil
			},
		},
		{
			Name: "omega_help", Title: "omega command help",
			Description: "The --help of an omega command, given as its words: '' for the top, 'disk limits', 'assist', 'agents kill'.",
			InputSchema: schema(map[string]any{"command": str("The command's words after omega")}),
			Annotations: readOnly("omega command help"),
			call: func(ctx context.Context, args map[string]any) (string, error) {
				c, err := word(args, "command", false)
				if err != nil {
					return "", err
				}
				words := strings.Fields(c)
				for _, w := range words {
					if !regexp.MustCompile(`^[a-z][a-z0-9-]*$`).MatchString(w) {
						return "", fmt.Errorf("%q is not a command word", w)
					}
				}
				return runOmega(ctx, append(words, "--help")...)
			},
		},
	}
}

func mcpToolNames() []string {
	var out []string
	for _, t := range mcpTools() {
		out = append(out, t.Name)
	}
	return out
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return home + p[1:]
	}
	return p
}

// runOmega runs this omega with args, no terminal and no colours, and
// returns what it printed. A failed command is returned as its output and
// the error, which the agent sees.
var runOmega = func(ctx context.Context, args ...string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = append(os.Environ(), "OMEGA_MCP=1", "NO_COLOR=1", "TERM=dumb")
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(stripANSI(string(out)))
	const max = 60000
	if len(text) > max {
		text = text[:max] + "\n… (cut)"
	}
	if err != nil {
		if text == "" {
			return "", err
		}
		return "", fmt.Errorf("%s\n(%v)", text, err)
	}
	if text == "" {
		text = "done"
	}
	return text, nil
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// serveMCP answers JSON-RPC messages, one per line, until in ends.
func serveMCP(ctx context.Context, in io.Reader, out io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	tools := mcpTools()
	byName := map[string]mcpTool{}
	for _, t := range tools {
		byName[t.Name] = t
	}
	enc := json.NewEncoder(out)
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			enc.Encode(rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}})
			continue
		}
		// A notification gets no answer.
		if len(req.ID) == 0 || string(req.ID) == "null" {
			continue
		}
		resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
		switch req.Method {
		case "initialize":
			var p struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			json.Unmarshal(req.Params, &p)
			v := mcpProtocol
			if p.ProtocolVersion != "" && p.ProtocolVersion <= mcpProtocol {
				v = p.ProtocolVersion
			}
			resp.Result = map[string]any{
				"protocolVersion": v,
				"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
				"serverInfo":      map[string]any{"name": "omega", "title": "omega", "version": version},
				"instructions": "omega maintains this computer. Use omega_docs before guessing an omega command. " +
					"omega_disk_limits and omega_assistants read; omega_set_limit and omega_apply_check change a tool's configuration " +
					"(ask the user first); omega_free_space removes files and only lists them unless dry_run is false.",
			}
		case "ping":
			resp.Result = map[string]any{}
		case "tools/list":
			resp.Result = map[string]any{"tools": tools}
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &p); err != nil {
				resp.Error = &rpcError{-32602, "invalid params"}
				break
			}
			t, ok := byName[p.Name]
			if !ok {
				resp.Error = &rpcError{-32602, "unknown tool " + p.Name}
				break
			}
			if p.Arguments == nil {
				p.Arguments = map[string]any{}
			}
			text, err := t.call(ctx, p.Arguments)
			isErr := err != nil
			if isErr {
				text = err.Error()
			}
			resp.Result = map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": isErr}
		default:
			resp.Error = &rpcError{-32601, "method not found: " + req.Method}
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return sc.Err()
}
