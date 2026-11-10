package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/steps"
)

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: i18n.T("cmd.doctor"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd.OutOrStdout())
		},
	}
}

// runDoctor checks the config, the key's shape, whether the provider answers
// and which catalogue steps are installed. It never sends a chat message.
func runDoctor(w io.Writer) error {
	problems := 0
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(w, i18n.T("doctor.config_bad", err.Error()))
		fmt.Fprint(w, doctorSteps(config.Default()))
		return fmt.Errorf("%s", i18n.T("doctor.failed", 1))
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(w, i18n.T("doctor.config_bad", err.Error()))
		problems++
	} else {
		fmt.Fprintln(w, i18n.T("doctor.config_ok", config.Path()))
	}
	problems += doctorProvider(w, cfg)
	fmt.Fprint(w, doctorSteps(cfg))
	if problems > 0 {
		return fmt.Errorf("%s", i18n.T("doctor.failed", problems))
	}
	return nil
}

func doctorProvider(w io.Writer, cfg *config.Config) int {
	info, ok := llm.Lookup(cfg.Provider)
	if !ok {
		fmt.Fprintln(w, i18n.T("doctor.provider_bad", i18n.T("err.unknown_provider", cfg.Provider, providerNames())))
		return 1
	}
	if info.Local {
		fmt.Fprintln(w, i18n.T("doctor.key_local"))
		base := cfg.BaseURLFor(info.Name)
		if base == "" {
			base = info.BaseURL
		}
		if base == "" {
			fmt.Fprintln(w, i18n.T("doctor.provider_bad", i18n.T("doctor.no_base")))
			return 1
		}
		ids, up := localModels(base)
		if !up {
			fmt.Fprintln(w, i18n.T("doctor.provider_bad", i18n.T("model.unreachable", info.Label, base)))
			return 1
		}
		fmt.Fprintln(w, i18n.T("doctor.provider_ok", info.Label))
		return doctorModels(w, cfg, info, ids)
	}

	key, src, err := getKey(info.Name)
	if err != nil {
		fmt.Fprintln(w, i18n.T("doctor.key_missing", info.Label))
		return 1
	}
	if keyLooksWrong(key) {
		fmt.Fprintln(w, i18n.T("doctor.key_bad", info.KeyURL))
		return 1
	}
	fmt.Fprintln(w, i18n.T("doctor.key_ok", string(src)))
	if pre := keyPrefix[info.Name]; pre != "" && !strings.HasPrefix(key, pre) {
		fmt.Fprintln(w, i18n.T("doctor.key_prefix", pre))
	}

	p, err := llm.New(info.Name, llm.Options{APIKey: key, BaseURL: cfg.BaseURLFor(info.Name)})
	if err != nil {
		fmt.Fprintln(w, i18n.T("doctor.provider_bad", err.Error()))
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	switch err := llm.Verify(ctx, p); {
	case err == nil:
		fmt.Fprintln(w, i18n.T("doctor.provider_ok", info.Label))
	case errors.Is(err, llm.ErrUnauthorized):
		fmt.Fprintln(w, i18n.T("doctor.key_rejected", info.Label))
		return 1
	default:
		fmt.Fprintln(w, i18n.T("doctor.provider_bad", err.Error()))
		return 1
	}
	if m := cfg.ModelFor(info.Name); m == "" || m == "auto" {
		loadModels(cfg, info, p, false)
		fast, smart := tiers(cfg, info)
		if live := livePicks(cfg, info.Name); live.Fast != "" {
			fmt.Fprintln(w, i18n.T("doctor.models_auto", fast, smart, modelStore.Fetched.Local().Format("2006-01-02")))
		} else {
			fmt.Fprintln(w, i18n.T("doctor.models_builtin", fast, smart))
		}
	}
	want := explicitModels(cfg, info.Name)
	if len(want) == 0 {
		return 0
	}
	models, err := p.Models(ctx)
	if err != nil {
		fmt.Fprintln(w, i18n.T("doctor.provider_bad", err.Error()))
		return 1
	}
	return doctorModels(w, cfg, info, models)
}

// localModels probes a local OpenAI-compatible /models endpoint. up is false
// only when the server cannot be reached or answers 5xx, matching the old
// reachability probe. A body that is not a model list leaves ids empty.
func localModels(base string) (ids []string, up bool) {
	c := http.Client{Timeout: 700 * time.Millisecond}
	resp, err := c.Get(strings.TrimRight(base, "/") + "/models")
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return nil, false
	}
	if resp.StatusCode >= 400 {
		return nil, true
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, true
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &out) != nil {
		return nil, true
	}
	for _, m := range out.Data {
		if id := strings.TrimSpace(m.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, true
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
			fmt.Fprintln(w, i18n.T("doctor.model_ok", name))
			continue
		}
		fmt.Fprintln(w, i18n.T("doctor.model_missing", name, info.Label))
		problems++
	}
	return problems
}

func doctorSteps(cfg *config.Config) string {
	installed, missing := 0, 0
	var b strings.Builder
	for _, s := range steps.All {
		mark := "·"
		if s.Available() {
			mark = "✓"
			installed++
		} else {
			missing++
		}
		line := fmt.Sprintf("  %s %s", mark, s.ID)
		if cfg.Skipped(s.ID) {
			line += " (" + i18n.T("doctor.skipped") + ")"
		}
		fmt.Fprintln(&b, line)
	}
	return i18n.T("doctor.steps", installed, missing) + "\n" + b.String()
}
