package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/customsteps"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/secrets"
	"github.com/lcajigasm/omega/internal/steps"
	"github.com/lcajigasm/omega/internal/ui"
)

func doctorCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "doctor",
		Short: i18n.T("cmd.doctor"),
		Long:  i18n.T("cmd.doctor_more"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !asJSON {
				return runDoctor(cmd.OutOrStdout())
			}
			info := doctorInfo()
			if err := writeJSON(cmd.OutOrStdout(), info); err != nil {
				return err
			}
			if !info.OK {
				return exitCode(1) // the document says what failed
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.doctor_json"))
	return c
}

// DoctorInfo is what `omega doctor --json` prints: the lines of the report,
// the facts a program would otherwise have to read out of them, and the
// steps of this machine.
type DoctorInfo struct {
	OK      bool     `json:"ok"`
	Lines   []string `json:"lines"`
	Version string   `json:"version"`
	Config  struct {
		Path  string `json:"path"`
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	} `json:"config"`
	Provider struct {
		Name string `json:"name"`
		OK   bool   `json:"ok"`
	} `json:"provider"`
	CustomStepProblems []string   `json:"custom_step_problems"`
	Steps              []StepInfo `json:"steps"`
}

// doctorInfo runs the doctor's checks and returns them as values.
func doctorInfo() DoctorInfo {
	var b strings.Builder
	facts, err := doctorCheck(&b, false)
	info := DoctorInfo{OK: err == nil, Lines: []string{}, Version: version, CustomStepProblems: []string{}, Steps: []StepInfo{}}
	for _, l := range strings.Split(b.String(), "\n") {
		if strings.TrimSpace(l) != "" {
			info.Lines = append(info.Lines, l)
		}
	}
	info.Config.Path, info.Config.OK, info.Config.Error = config.Path(), facts.configErr == "", facts.configErr
	info.Provider.Name, info.Provider.OK = facts.provider, facts.providerOK
	info.CustomStepProblems = append(info.CustomStepProblems, facts.custom...)
	if list, err := StepList(); err == nil {
		info.Steps = list
	}
	return info
}

// doctorFacts is what the doctor found, beside the lines it wrote.
type doctorFacts struct {
	configErr  string // why the configuration is not usable, or ""
	provider   string
	providerOK bool
	custom     []string // steps.d files and steps that were left out
}

// runDoctor checks the config, the key's shape, whether the provider answers
// and which catalogue steps are installed. It never sends a chat message.
// latestRelease asks GitHub for the tag of the newest omega release. Tests
// replace it; an error or an empty answer just leaves the comparison out.
var latestRelease = func() string {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/lcajigasm/omega/releases/latest", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "omega")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var out struct {
		Tag string `json:"tag_name"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil {
		return ""
	}
	return out.Tag
}

// versionParts reads "v0.13.0" (or "0.13.0-4-gabc") as its three numbers.
func versionParts(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// doctorVersion says which omega this is and whether a newer release exists.
// An old omega is not a failure, but it is the usual reason a documented
// command "does not exist".
func doctorVersion(w io.Writer) {
	mine, ok := versionParts(version)
	if !ok {
		doctorLine(w, ui.MarkInfo, i18n.T("doctor.version_unknown", version))
		return
	}
	latest := latestRelease()
	theirs, ok := versionParts(latest)
	switch {
	case !ok:
		doctorLine(w, ui.MarkInfo, i18n.T("doctor.version_unknown", version))
	case theirs[0] > mine[0] || (theirs[0] == mine[0] && (theirs[1] > mine[1] || (theirs[1] == mine[1] && theirs[2] > mine[2]))):
		doctorLine(w, ui.MarkWarn, i18n.T("doctor.version_old", version, latest))
	default:
		doctorLine(w, ui.MarkOK, i18n.T("doctor.version_ok", version))
	}
}

func runDoctor(w io.Writer) error {
	ui.NewReport(w).Title("omega doctor", i18n.T("doctor.subtitle"))
	err := doctorReport(w, true)
	if err == nil {
		ui.NewReport(w).Hint(i18n.T("doctor.all_ok"))
	}
	return err
}

// doctorReport writes the checks; withSteps adds the list of steps, which a
// frontend that draws its own leaves out.
func doctorReport(w io.Writer, withSteps bool) error {
	_, err := doctorCheck(w, withSteps)
	return err
}

// doctorCheck writes the report and returns what it found.
func doctorCheck(w io.Writer, withSteps bool) (doctorFacts, error) {
	var facts doctorFacts
	problems := 0
	doctorVersion(w)
	cfg, err := config.Load()
	if err != nil {
		facts.configErr = err.Error()
		doctorLine(w, ui.MarkErr, i18n.T("doctor.config_bad", err.Error()))
		if withSteps {
			fmt.Fprint(w, doctorSteps(config.Default()))
		}
		return facts, fmt.Errorf("%s", i18n.N("doctor.failed", 1))
	}
	if err := cfg.Validate(); err != nil {
		facts.configErr = err.Error()
		doctorLine(w, ui.MarkErr, i18n.T("doctor.config_bad", err.Error()))
		problems++
	} else {
		doctorLine(w, ui.MarkOK, i18n.T("doctor.config_ok", config.Path()))
	}
	// A key omega does not know is most likely a typo whose setting stays
	// the default: a warning, not a failure.
	for _, k := range unknownConfigKeys() {
		doctorLine(w, ui.MarkWarn, i18n.T("config.unknown_key", k))
	}
	facts.provider = cfg.Provider
	providerProblems := doctorProvider(w, cfg)
	facts.providerOK = providerProblems == 0
	problems += providerProblems
	// A step of the user's own that omega left out never runs: say which.
	_, facts.custom = customsteps.Load()
	for _, p := range facts.custom {
		doctorLine(w, ui.MarkErr, i18n.T("doctor.custom_bad", p))
		problems++
	}
	if withSteps {
		fmt.Fprint(w, doctorSteps(cfg))
	}
	if problems > 0 {
		return facts, fmt.Errorf("%s", i18n.N("doctor.failed", problems))
	}
	return facts, nil
}

// keyStore is where omega would save a key here. Tests replace it.
var keyStore = secrets.Store

func doctorProvider(w io.Writer, cfg *config.Config) int {
	info, ok := llm.Lookup(cfg.Provider)
	if !ok {
		doctorLine(w, ui.MarkErr, i18n.T("doctor.provider_bad", i18n.T("err.unknown_provider", cfg.Provider, providerNames())))
		return 1
	}
	if info.Local {
		doctorLine(w, ui.MarkOK, i18n.T("doctor.key_local"))
		base := cfg.BaseURLFor(info.Name)
		if base == "" {
			base = info.BaseURL
		}
		if base == "" {
			doctorLine(w, ui.MarkErr, i18n.T("doctor.provider_bad", i18n.T("doctor.no_base")))
			return 1
		}
		ids, up := localModels(base)
		if !up {
			doctorLine(w, ui.MarkErr, i18n.T("doctor.provider_bad", i18n.T("model.unreachable", info.Label, base)))
			return 1
		}
		doctorLine(w, ui.MarkOK, i18n.T("doctor.provider_ok", info.Label))
		return doctorModels(w, cfg, info, ids)
	}

	key, src, err := getKey(info.Name)
	if err != nil {
		doctorLine(w, ui.MarkErr, i18n.T("doctor.key_missing", info.Label))
		if store := keyStore(); store != "" {
			doctorLine(w, ui.MarkInfo, i18n.T("doctor.key_store", secrets.Describe(store)))
		}
		return 1
	}
	if keyLooksWrong(key) {
		doctorLine(w, ui.MarkErr, i18n.T("doctor.key_bad", info.KeyURL))
		return 1
	}
	doctorLine(w, ui.MarkOK, i18n.T("doctor.key_ok", secrets.Describe(src)))
	if pre := keyPrefix[info.Name]; pre != "" && !strings.HasPrefix(key, pre) {
		doctorLine(w, ui.MarkWarn, i18n.T("doctor.key_prefix", pre))
	}

	p, err := llm.New(info.Name, llm.Options{APIKey: key, BaseURL: cfg.BaseURLFor(info.Name)})
	if err != nil {
		doctorLine(w, ui.MarkErr, i18n.T("doctor.provider_bad", err.Error()))
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	switch err := llm.Verify(ctx, p); {
	case err == nil:
		doctorLine(w, ui.MarkOK, i18n.T("doctor.provider_ok", info.Label))
	case errors.Is(err, llm.ErrUnauthorized):
		doctorLine(w, ui.MarkErr, i18n.T("doctor.key_rejected", info.Label))
		return 1
	default:
		doctorLine(w, ui.MarkErr, i18n.T("doctor.provider_bad", err.Error()))
		return 1
	}
	if m := cfg.ModelFor(info.Name); m == "" || m == "auto" {
		loadModels(cfg, info, p, false)
		fast, smart := tiers(cfg, info)
		if live := livePicks(cfg, info.Name); live.Fast != "" {
			doctorLine(w, ui.MarkOK, i18n.T("doctor.models_auto", fast, smart, modelStore.Fetched.Local().Format("2006-01-02")))
		} else {
			doctorLine(w, ui.MarkOK, i18n.T("doctor.models_builtin", fast, smart))
		}
	}
	want := explicitModels(cfg, info.Name)
	if len(want) == 0 {
		return 0
	}
	models, err := p.Models(ctx)
	if err != nil {
		doctorLine(w, ui.MarkErr, i18n.T("doctor.provider_bad", err.Error()))
		return 1
	}
	return doctorModels(w, cfg, info, models)
}

// explicitModels lists model names the user typed. auto and the built-in
// defaults are left out: doctor only checks a name that would be sent as is.
func explicitModels(cfg *config.Config, provider string) []string {
	pc := cfg.Providers[provider]
	var names []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || strings.EqualFold(s, "auto") {
			return
		}
		for _, n := range names {
			if n == s {
				return
			}
		}
		names = append(names, s)
	}
	if cfg.Provider == provider {
		add(cfg.Model)
	}
	add(pc.Model)
	add(pc.FastModel)
	add(pc.SmartModel)
	return names
}

// doctorModels fails when a configured name is missing from a non-empty list.
// An empty list is not a failure: the server did not give names to compare.
func doctorModels(w io.Writer, cfg *config.Config, info llm.Info, ids []string) int {
	want := explicitModels(cfg, info.Name)
	if len(want) == 0 || len(ids) == 0 {
		return 0
	}
	have := map[string]bool{}
	for _, id := range ids {
		have[id] = true
	}
	problems := 0
	for _, name := range want {
		if have[name] {
			doctorLine(w, ui.MarkOK, i18n.T("doctor.model_ok", name))
			continue
		}
		doctorLine(w, ui.MarkWarn, i18n.T("doctor.model_missing", name, info.Label))
		problems++
	}
	return problems
}

// stepCounts is how many steps are installed here and how many are not.
func stepCounts(*config.Config) (installed, missing int) {
	for _, s := range steps.All {
		if s.Available() {
			installed++
		} else {
			missing++
		}
	}
	return
}

func doctorSteps(cfg *config.Config) string {
	installed, missing := 0, 0
	var cells []string
	width := 0
	for _, s := range steps.All {
		mark := "·"
		if s.Available() {
			mark = "✓"
			installed++
		} else {
			missing++
		}
		cell := mark + " " + s.ID
		if cfg.Skipped(s.ID) {
			cell += " (" + i18n.T("doctor.skipped") + ")"
		}
		cells = append(cells, cell)
		width = max(width, len([]rune(cell)))
	}
	// A grid, not one line per step: 80 steps on one screen.
	cols := max(1, min(4, 96/(width+2)))
	rows := (len(cells) + cols - 1) / cols
	var b strings.Builder
	for r := 0; r < rows; r++ {
		b.WriteString("   ")
		for c := 0; c < cols; c++ {
			i := c*rows + r
			if i >= len(cells) {
				break
			}
			cell := cells[i]
			if c < cols-1 {
				cell += strings.Repeat(" ", width+2-len([]rune(cell)))
			}
			b.WriteString(cell)
		}
		b.WriteString("\n")
	}
	var out strings.Builder
	for _, l := range strings.Split(b.String(), "\n") {
		if l = strings.TrimRight(l, " "); l != "" {
			out.WriteString(l + "\n")
		}
	}
	return "  ✓ " + i18n.T("doctor.steps", installed, missing) + "\n" + out.String()
}

// doctorLine writes one check with its mark: "✓ key: ok (macOS Keychain)".
func doctorLine(w io.Writer, m ui.Mark, text string) {
	r := ui.NewReport(w)
	label, rest, ok := strings.Cut(text, ":")
	if ok && len([]rune(label)) <= 14 {
		text = r.Bold(label+":") + rest
	}
	fmt.Fprintln(w, "  "+r.Glyph(m)+" "+text)
}
