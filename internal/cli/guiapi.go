package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/steps"
	"github.com/lcajigasm/omega/internal/ui"
)

// What the desktop app asks beyond starting a session: the same settings,
// steps, transcripts, checks and notes the commands print, as values a
// frontend can draw.

// SetConfig changes one setting, as `omega config set` does. A new language
// takes effect at once.
func SetConfig(k, v string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	switch k {
	case "provider":
		if err := validProvider(v); err != nil {
			return err
		}
		cfg.Provider = v
		cfg.Model = cfg.Providers[v].Model
	case "model":
		cfg.Model = v
		pc := cfg.Providers[cfg.Provider]
		pc.Model = v
		cfg.Providers[cfg.Provider] = pc
	case "effort":
		cfg.Effort = v
	case "approval":
		if v != config.ApproveDangerous && v != config.ApproveAll && v != config.ApproveNever {
			return fmt.Errorf("%s", i18n.T("config.bad_approval"))
		}
		cfg.Approval = v
	case "language":
		if v != "auto" && i18n.Normalize(v) == "" {
			return fmt.Errorf("%s", i18n.T("config.bad_language", v))
		}
		cfg.Language = v
	case "command_timeout":
		if _, err := time.ParseDuration(v); err != nil {
			return fmt.Errorf("%s", i18n.T("config.bad_duration", err))
		}
		cfg.Timeout = v
	case "model_catalog":
		cfg.Catalog = v
	case "skip_steps":
		cfg.SkipSteps = nil
		for _, s := range strings.Split(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				cfg.SkipSteps = append(cfg.SkipSteps, s)
			}
		}
	default:
		return fmt.Errorf("%s", i18n.T("config.unknown_key", k))
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	if k == "language" {
		i18n.Init(cfg.Language)
	}
	return nil
}

// Settings are the values of the settings screen.
type Settings struct {
	Language string `json:"language"` // "auto" or a language code
	Approval string `json:"approval"`
	Effort   string `json:"effort"` // "" leaves it to the provider
	Timeout  string `json:"timeout"`
	Catalog  bool   `json:"catalog"` // the public model catalogue may be downloaded
	Model    string `json:"model"`   // "auto" or a fixed model
	Path     string `json:"path"`    // where the configuration file is
}

// GetSettings reads the settings.
func GetSettings() (Settings, error) {
	cfg, err := config.Load()
	if err != nil {
		return Settings{}, err
	}
	model := cfg.ModelFor(cfg.Provider)
	if model == "" {
		model = "auto"
	}
	return Settings{Language: cfg.Language, Approval: cfg.Approval, Effort: strings.ToLower(cfg.Effort), Timeout: cfg.Timeout,
		Catalog: cfg.Catalog != config.CatalogOff, Model: model, Path: config.Path()}, nil
}

// ModelChoice is one model the configured provider can be set to.
type ModelChoice struct {
	ID     string `json:"id"` // "auto" or the model's name
	Label  string `json:"label"`
	Detail string `json:"detail"`
	Badge  string `json:"badge"` // price, when known
}

// ModelChoices lists the models of the configured provider, best first. It
// asks the provider, so it needs the network and a key. The list still holds
// the current model when the provider cannot be reached.
func ModelChoices() ([]ModelChoice, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	info, ok := llm.Lookup(cfg.Provider)
	if !ok {
		return nil, fmt.Errorf("%s", i18n.T("gui.unknown_prov", cfg.Provider))
	}
	var opts []ui.Option
	p, err := newProvider(cfg, info, "")
	if err == nil {
		opts, _, _, err = modelOptions(cfg, info, p)
	}
	out := []ModelChoice{}
	seen := map[string]bool{}
	for _, o := range opts {
		seen[o.ID] = true
		out = append(out, ModelChoice{ID: o.ID, Label: o.Label, Detail: o.Detail, Badge: o.Badge})
	}
	if !seen["auto"] {
		out = append([]ModelChoice{{ID: "auto", Label: i18n.T("model.auto")}}, out...)
	}
	if cur := cfg.ModelFor(cfg.Provider); cur != "" && cur != "auto" && !seen[cur] {
		out = append(out, ModelChoice{ID: cur, Label: cur})
	}
	return out, err
}

// SetModel fixes the configured provider's model; "auto" picks per task.
func SetModel(choice string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if choice = strings.TrimSpace(choice); choice == "" {
		choice = "auto"
	}
	return saveModel(cfg, cfg.Provider, choice)
}

// StepInfo is one maintenance step of this machine's catalogue.
type StepInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Category  string `json:"category"` // heading, translated
	Risk      string `json:"risk"`     // translated
	Dangerous bool   `json:"dangerous"`
	Installed bool   `json:"installed"`
	Skipped   bool   `json:"skipped"` // the user turned it off
	// The same in words that do not change with the language, for
	// `omega steps --json` and any program that reads it.
	CategoryID string `json:"category_id"` // check, update, clean, backend
	RiskID     string `json:"risk_id"`     // read-only, normal, dangerous
	Command    string `json:"command"`
	Available  bool   `json:"available"` // what it needs is installed
}

// StepList is what `omega steps` prints.
func StepList() ([]StepInfo, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	out := []StepInfo{}
	for _, s := range steps.Grouped(steps.All) {
		out = append(out, StepInfo{ID: s.ID, Name: i18n.TOr("step."+s.ID, s.Name), Category: i18n.T("cat." + s.Category),
			Risk: ui.RiskName(s.Risk), Dangerous: ui.RiskName(s.Risk) == i18n.T("risk.dangerous"),
			Installed: s.Available(), Skipped: cfg.Skipped(s.ID),
			CategoryID: s.Category, RiskID: s.Risk.String(), Command: s.Command, Available: s.Available()})
	}
	return out, nil
}

// SkipStep turns a step off (skip) or back on, in skip_steps.
func SkipStep(id string, skip bool) error {
	if !steps.Known(id) {
		return fmt.Errorf("%s", i18n.T("steps.unknown", id))
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	kept := cfg.SkipSteps[:0:0]
	for _, s := range cfg.SkipSteps {
		if s != id {
			kept = append(kept, s)
		}
	}
	if skip {
		kept = append(kept, id)
	}
	cfg.SkipSteps = kept
	return cfg.Save()
}

// RunInfo is one saved transcript.
type RunInfo struct {
	Name  string `json:"name"`
	Time  string `json:"time"` // RFC 3339
	Ago   string `json:"ago"`
	Size  int64  `json:"size"`
	Title string `json:"title"` // provider and model, from its first line
}

// RunList lists the saved transcripts, newest first.
func RunList() ([]RunInfo, error) {
	logs, err := agent.ListRunLogs()
	if err != nil {
		return nil, err
	}
	out := []RunInfo{}
	for _, l := range logs {
		out = append(out, RunInfo{Name: l.Name, Time: l.ModTime.Format(time.RFC3339), Ago: ago(l.ModTime), Size: l.Size, Title: runTitle(l.Path)})
	}
	return out, nil
}

// runTitle is what follows the dash in a transcript's first line:
// "omega run <time> — <provider> / <model>".
func runTitle(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, 256)
	n, _ := f.Read(buf)
	line, _, _ := strings.Cut(string(buf[:n]), "\n")
	_, title, _ := strings.Cut(line, " — ")
	return strings.TrimSpace(title)
}

// maxRunText is how much of a transcript the app shows: its end.
const maxRunText = 400 << 10

// RunText returns a transcript by file name; a long one keeps its end.
func RunText(name string) (string, error) {
	_, b, err := agent.ReadRunLog(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%s", i18n.T("logs.missing", name))
		}
		return "", err
	}
	if len(b) > maxRunText {
		b = b[len(b)-maxRunText:]
		if i := strings.IndexByte(string(b), '\n'); i >= 0 {
			b = b[i+1:]
		}
		return "…\n" + string(b), nil
	}
	return string(b), nil
}

// LastRun says how long ago the last session was, or "" when there is none.
func LastRun() string {
	if logs, err := agent.ListRunLogs(); err == nil && len(logs) > 0 {
		return ago(logs[0].ModTime)
	}
	return ""
}

// Check is what `omega doctor` found, without the list of steps.
type Check struct {
	OK    bool     `json:"ok"`
	Lines []string `json:"lines"`
}

// RunCheck checks the version, the configuration, the provider and its key.
func RunCheck() Check {
	var b strings.Builder
	err := doctorReport(&b, false)
	c := Check{OK: err == nil, Lines: []string{}}
	for _, l := range strings.Split(strings.TrimSpace(b.String()), "\n") {
		if strings.TrimSpace(l) != "" {
			c.Lines = append(c.Lines, l)
		}
	}
	if err != nil {
		c.Lines = append(c.Lines, err.Error())
	}
	return c
}

// Notes is what omega has written down about this machine between runs.
func Notes() string {
	b, _ := os.ReadFile(config.NotesPath())
	return strings.TrimSpace(string(b))
}

// ClearNotes forgets the notes.
func ClearNotes() error {
	err := os.Remove(config.NotesPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
