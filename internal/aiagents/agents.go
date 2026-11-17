package aiagents

import (
	"path/filepath"
	"strings"
)

// Agent is an AI coding agent omega knows: how its process looks in the
// process list and how it marks the commands it runs.
type Agent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Exes are the names of its executable, as `ps` shows them.
	Exes []string `json:"-"`
	// Scripts are paths a runtime (node, bun, python) runs it from: a
	// process is the agent when its first argument contains one of them.
	Scripts []string `json:"-"`
	// Marker is the variable it sets in the environment of every command
	// it runs, and Value the value it sets ("" for any value). Empty when
	// it marks nothing: then what it leaves behind cannot be found.
	Marker string `json:"-"`
	Value  string `json:"-"`
	// SessionVar and PIDVar hold the session's ID and the agent's process
	// in that environment, when it sets them.
	SessionVar string `json:"-"`
	PIDVar     string `json:"-"`
	// Records says it keeps a record per running process with the
	// session's status: only Claude Code does.
	Records bool `json:"-"`
	// Services are words of the command line of its background service
	// (`codex app-server`, `opencode serve`): the service owns the
	// commands of the sessions that talk to it, and is not ended from here.
	Services []string `json:"-"`
}

// service reports whether a command line is the agent's background service.
func (a *Agent) service(command string) bool {
	f := strings.Fields(command)
	for _, w := range f[min(1, len(f)):] {
		for _, sv := range a.Services {
			if w == sv {
				return true
			}
		}
	}
	return false
}

// marks reports whether an environment carries the agent's marker.
func (a *Agent) marks(env map[string]string) bool {
	if a.Marker == "" {
		return false
	}
	v, ok := env[a.Marker]
	return ok && v != "" && (a.Value == "" || v == a.Value)
}

// Catalogue is the agents omega knows, with the variables each one sets in
// the environment of the commands it runs, as their source code sets them
// (Cursor's from its published bundle, Copilot's from its changelog). Amp
// and Aider mark nothing that can be confirmed: their open sessions show,
// what they leave behind does not.
//
// The order matters when an agent runs inside another and a command
// carries both markers and neither agent runs any more: the later one is
// taken as the inner one. Kilo is a fork of opencode and also sets
// OPENCODE=1, so it comes after it.
var Catalogue = []Agent{
	{ID: "claude", Name: "Claude Code", Exes: []string{"claude"}, Scripts: []string{"@anthropic-ai/claude-code/"},
		Marker: "CLAUDECODE", Value: "1", SessionVar: "CLAUDE_CODE_SESSION_ID", PIDVar: "CLAUDE_PID", Records: true},
	{ID: "codex", Name: "Codex", Exes: []string{"codex"}, Scripts: []string{"@openai/codex/"},
		Marker: "CODEX_THREAD_ID", SessionVar: "CODEX_THREAD_ID", Services: []string{"app-server"}},
	{ID: "gemini", Name: "Gemini CLI", Exes: []string{"gemini"}, Scripts: []string{"@google/gemini-cli/", "/bin/gemini"},
		Marker: "GEMINI_CLI", Value: "1"},
	{ID: "qwen", Name: "Qwen Code", Exes: []string{"qwen"}, Scripts: []string{"@qwen-code/qwen-code/", "/bin/qwen"},
		Marker: "QWEN_CODE", Value: "1", SessionVar: "QWEN_CODE_SESSION_ID"},
	{ID: "opencode", Name: "opencode", Exes: []string{"opencode"}, Scripts: []string{"opencode-ai/", "@opencode/cli"},
		Marker: "OPENCODE", Value: "1", SessionVar: "OPENCODE_SESSION_ID", PIDVar: "OPENCODE_PID", Services: []string{"serve"}},
	{ID: "kilo", Name: "Kilo Code", Exes: []string{"kilo"}, Scripts: []string{"@kilocode/cli"},
		Marker: "KILO", Value: "1", PIDVar: "KILO_PID"},
	{ID: "cursor", Name: "Cursor CLI", Exes: []string{"cursor-agent"}, Scripts: []string{"/cursor-agent/"},
		Marker: "CURSOR_AGENT", Value: "1", SessionVar: "CURSOR_CONVERSATION_ID"},
	{ID: "copilot", Name: "GitHub Copilot CLI", Exes: []string{"copilot"}, Scripts: []string{"@github/copilot"},
		Marker: "COPILOT_CLI", Value: "1", SessionVar: "COPILOT_AGENT_SESSION_ID"},
	{ID: "crush", Name: "Crush", Exes: []string{"crush"}, Marker: "CRUSH", Value: "1"},
	{ID: "goose", Name: "Goose", Exes: []string{"goose", "goosed"}, Marker: "GOOSE_TERMINAL", Value: "1", SessionVar: "AGENT_SESSION_ID"},
	{ID: "amp", Name: "Amp", Exes: []string{"amp"}, Scripts: []string{"@sourcegraph/amp", "@ampcode/cli"}},
	{ID: "aider", Name: "Aider", Exes: []string{"aider"}, Scripts: []string{"/bin/aider", "aider-chat"}},
}

// ByID finds an agent of the catalogue.
func ByID(id string) (*Agent, bool) {
	for i := range Catalogue {
		if Catalogue[i].ID == id {
			return &Catalogue[i], true
		}
	}
	return nil, false
}

// NameOf is the agent's name, or the ID when it is not in the catalogue.
func NameOf(id string) string {
	if a, ok := ByID(id); ok {
		return a.Name
	}
	return id
}

// envKeys are the variables read from environments: every marker, session
// and PID variable of the catalogue, and nothing else.
func envKeys() map[string]bool {
	keys := map[string]bool{}
	for _, a := range Catalogue {
		for _, k := range []string{a.Marker, a.SessionVar, a.PIDVar} {
			if k != "" {
				keys[k] = true
			}
		}
	}
	return keys
}

// runtimes run agents written as scripts.
var runtimes = map[string]bool{"node": true, "bun": true, "deno": true, "python": true, "python3": true, "Python": true}

// identify says which agent a process is, from its executable and command
// line, or nil. The name must match exactly: the Claude desktop app's
// process is "Claude", not the agent.
func identify(exe, command string) *Agent {
	name := strings.TrimSuffix(filepath.Base(exe), ".exe")
	f := strings.Fields(command)
	// A launcher may run the runtime under the agent's name (`exec -a`):
	// then the first word of the command line is the agent's.
	argv0 := ""
	if len(f) > 0 {
		argv0 = filepath.Base(f[0])
	}
	for i := range Catalogue {
		for _, e := range Catalogue[i].Exes {
			if name == e || (runtimes[name] && argv0 == e) {
				return &Catalogue[i]
			}
		}
	}
	if !runtimes[name] && !strings.HasPrefix(name, "python3.") {
		return nil
	}
	// The script is the first argument that is not a flag of the runtime.
	script := ""
	for _, arg := range f[min(1, len(f)):] {
		if !strings.HasPrefix(arg, "-") {
			script = arg
			break
		}
	}
	if script == "" {
		return nil
	}
	for i := range Catalogue {
		for _, sc := range Catalogue[i].Scripts {
			if strings.Contains(script, sc) {
				return &Catalogue[i]
			}
		}
	}
	return nil
}
