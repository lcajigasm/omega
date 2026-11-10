package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/models"
	"github.com/lcajigasm/omega/internal/ui"
)

// modelPlan is the model choice for one run.
type modelPlan struct {
	Model    string
	Escalate string   // switch to this model when a step fails; "" for none
	Choices  []string // models the user can switch to during the run
}

// tiers returns the fast and smart models for a provider: the picks from the
// live catalogue when there is one, else the names built into omega, and in
// both cases the user's overrides on top.
func tiers(cfg *config.Config, info llm.Info) (fast, smart string) {
	pc := cfg.Providers[info.Name]
	fast, smart = info.FastModel, info.SmartModel
	if live := livePicks(cfg, info.Name); live.Fast != "" {
		fast, smart = live.Fast, live.Smart
	}
	if pc.FastModel != "" {
		fast = pc.FastModel
	}
	if pc.SmartModel != "" {
		smart = pc.SmartModel
	}
	if fast == "" {
		fast = smart
	}
	if smart == "" {
		smart = fast
	}
	return fast, smart
}

// planModel decides which model runs a task. An explicit model (flag or
// config) is used as is. Otherwise tasks start on the fast tier and switch to
// the smart tier on the first failing step; fix, which is all diagnosis,
// starts on the smart tier.
func planModel(cfg *config.Config, info llm.Info, flagModel, task string) (modelPlan, error) {
	plan, err := pickModel(cfg, info, flagModel, task)
	if err != nil {
		return plan, err
	}
	fast, smart := tiers(cfg, info)
	plan.Choices = uniqueModels(plan.Model, plan.Escalate, fast, smart)
	return plan, nil
}

// uniqueModels drops empty and repeated names, keeping the order.
func uniqueModels(names ...string) []string {
	var out []string
	seen := map[string]bool{}
	for _, n := range names {
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

func pickModel(cfg *config.Config, info llm.Info, flagModel, task string) (modelPlan, error) {
	if flagModel != "" {
		return modelPlan{Model: flagModel}, nil
	}
	if m := cfg.ModelFor(info.Name); m != "" && m != "auto" {
		return modelPlan{Model: m}, nil
	}
	fast, smart := tiers(cfg, info)
	if fast == "" {
		return modelPlan{}, fmt.Errorf("%s", i18n.T("model.need_local", info.Name))
	}
	if task == agent.TaskFix {
		return modelPlan{Model: smart}, nil
	}
	if smart != fast {
		return modelPlan{Model: fast, Escalate: smart}, nil
	}
	return modelPlan{Model: fast}, nil
}

// probeLocal reports whether a local OpenAI-compatible server answers.
func probeLocal(baseURL string) bool {
	c := http.Client{Timeout: 700 * time.Millisecond}
	resp, err := c.Get(strings.TrimRight(baseURL, "/") + "/models")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

func priceBadge(provider, model string) string {
	p, ok := llm.PriceFor(provider, model)
	if !ok {
		return ""
	}
	if p.Input == 0 && p.Output == 0 {
		return i18n.T("model.free")
	}
	badge := fmt.Sprintf("$%g / $%g per M", p.Input, p.Output)
	if s := scores(provider)[model]; s > 0 {
		badge = fmt.Sprintf("%s %.0f · %s", i18n.T("models.col_score"), s, badge)
	}
	return badge
}

// scores are the catalogue scores of a provider's models, by name.
func scores(provider string) map[string]float64 {
	out := map[string]float64{}
	if modelStore == nil {
		return out
	}
	for _, c := range modelStore.For(provider, nil, time.Now()) {
		out[c.Native] = c.Score
	}
	return out
}

// byScore puts the models the catalogue scores first, best first, and keeps
// the rest in their order.
func byScore(cfg *config.Config, info llm.Info, names []string) []string {
	if modelStore == nil || cfg.Catalog == config.CatalogOff {
		return names
	}
	score := map[string]float64{}
	for _, c := range modelStore.For(info.Name, names, time.Now()) {
		score[c.Native] = c.Score
	}
	out := append([]string(nil), names...)
	sort.SliceStable(out, func(i, j int) bool { return score[out[i]] > score[out[j]] })
	return out
}

// modelOptions lists what a provider's model can be set to: automatic when
// the provider has tiers, then every model it offers, best first. listErr is
// why the provider's own list could not be read, if it could not.
func modelOptions(cfg *config.Config, info llm.Info, p llm.Provider) (opts []ui.Option, fast, smart string, listErr error) {
	fast, smart = tiers(cfg, info)
	seen := map[string]bool{}
	if fast != "" {
		opts = append(opts, ui.Option{ID: "auto", Label: i18n.T("model.auto"),
			Detail: i18n.T("model.auto_detail", fast, smart), Risk: "read-only"})
		opts = append(opts, ui.Option{ID: fast, Label: fast, Detail: i18n.T("model.fast"), Badge: priceBadge(info.Name, fast)})
		seen[fast] = true
		if !seen[smart] {
			opts = append(opts, ui.Option{ID: smart, Label: smart, Detail: i18n.T("model.smart"), Badge: priceBadge(info.Name, smart)})
			seen[smart] = true
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	models, listErr := p.Models(ctx)
	cancel()
	sort.Strings(models)
	models = byScore(cfg, info, models)
	for _, m := range models {
		if seen[m] || strings.HasSuffix(m, ":batch") {
			continue
		}
		seen[m] = true
		badge := priceBadge(info.Name, m)
		if info.Local {
			badge = i18n.T("model.free")
		}
		opts = append(opts, ui.Option{ID: m, Label: m, Badge: badge})
	}
	return opts, fast, smart, listErr
}

// saveModel fixes a provider's model; "auto" lets omega pick per task.
func saveModel(cfg *config.Config, provider, choice string) error {
	pc := cfg.Providers[provider]
	if choice == "auto" {
		pc.Model = ""
	} else {
		pc.Model = choice
	}
	cfg.Providers[provider] = pc
	if cfg.Provider == provider {
		cfg.Model = pc.Model
	}
	return cfg.Save()
}

// chooseModel shows the model card for a provider and saves the choice.
func chooseModel(cfg *config.Config, info llm.Info, p llm.Provider) error {
	opts, fast, smart, err := modelOptions(cfg, info, p)
	if err != nil {
		fmt.Println(i18n.T("setup.models_failed", err))
	}
	if len(opts) == 0 {
		return fmt.Errorf("%s", i18n.T("model.need_local", info.Name))
	}
	tty := term.IsTerminal(int(os.Stdin.Fd()))
	ids, err := ui.Pick(i18n.T("model.title", info.Label), i18n.T("model.hint"), opts, false, tty, os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	choice := ids[0]
	if err := saveModel(cfg, info.Name, choice); err != nil {
		return err
	}
	shown := choice
	if choice == "auto" {
		shown = i18n.T("model.auto") + " (" + fast + " → " + smart + ")"
	}
	fmt.Println(okText(i18n.T("model.saved", info.Name, shown)))
	return nil
}

func okText(s string) string { return "✓ " + s }

func modelCmd() *cobra.Command {
	var provider string
	c := &cobra.Command{
		Use:   "model [model|auto]",
		Short: i18n.T("cmd.model"),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if provider == "" {
				provider = cfg.Provider
			}
			info, ok := llm.Lookup(provider)
			if !ok {
				return fmt.Errorf("%s", i18n.T("err.unknown_provider", provider, providerNames()))
			}
			if len(args) == 1 {
				pc := cfg.Providers[provider]
				pc.Model = args[0]
				if args[0] == "auto" {
					pc.Model = ""
				}
				cfg.Providers[provider] = pc
				if cfg.Provider == provider {
					cfg.Model = pc.Model
				}
				if err := cfg.Save(); err != nil {
					return err
				}
				fmt.Println(okText(i18n.T("model.saved", provider, args[0])))
				return nil
			}
			p, err := newProvider(cfg, info, "")
			if err != nil {
				return err
			}
			loadModels(cfg, info, p, false)
			err = chooseModel(cfg, info, p)
			if errors.Is(err, ui.ErrCancelled) {
				return nil
			}
			return err
		},
	}
	c.Flags().StringVar(&provider, "provider", "", i18n.T("flag.provider"))
	return c
}

// newProvider builds a provider with its key (none for local servers).
func newProvider(cfg *config.Config, info llm.Info, effort string) (llm.Provider, error) {
	opts := llm.Options{BaseURL: cfg.BaseURLFor(info.Name), Effort: effort}
	if info.Local {
		if key, _, err := getKey(info.Name); err == nil {
			opts.APIKey = key
		}
		base := opts.BaseURL
		if base == "" {
			base = info.BaseURL
		}
		if base != "" && !probeLocal(base) {
			return nil, fmt.Errorf("%s", i18n.T("model.unreachable", info.Label, base))
		}
	} else {
		key, _, err := getKey(info.Name)
		if err != nil {
			return nil, err
		}
		opts.APIKey = key
	}
	return llm.New(info.Name, opts)
}

// modelStore is the live catalogue once a command has loaded it. It stays nil
// in tests and in commands that never choose a model.
var modelStore *models.Store

// catalogDisabled keeps the tests off the network.
var catalogDisabled bool

func modelStorePath() string { return filepath.Join(config.StateDir(), "models.json") }

// livePicks are the automatic choices for a provider from the catalogue.
func livePicks(cfg *config.Config, provider string) models.Picks {
	if modelStore == nil || cfg.Catalog == config.CatalogOff {
		return models.Picks{}
	}
	return modelStore.Picks[provider]
}

// loadModels reads the stored catalogue and, when it is a day old (or force
// is set), downloads it again and picks the models for the provider. p may be
// nil when the provider could not be built; the stored picks are still used.
// A failed download is not an error for a run: omega falls back to what it
// has. The error is returned for the commands that report it.
func loadModels(cfg *config.Config, info llm.Info, p llm.Provider, force bool) error {
	if info.Local || cfg.Catalog == config.CatalogOff || catalogDisabled {
		return nil
	}
	path := modelStorePath()
	store := models.Load(path)
	modelStore = store
	llm.LivePrice = func(provider, model string) (llm.Price, bool) {
		in, out, ok := store.Price(provider, model)
		return llm.Price{Input: in, Output: out}, ok
	}
	if p == nil {
		return nil
	}
	wait := 6 * time.Second
	if force {
		wait = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	fetched, before := store.Fetched, store.Picks[info.Name]
	err := store.Update(ctx, info.Name, p.Models, force, time.Now())
	if !store.Fetched.Equal(fetched) || store.Picks[info.Name] != before {
		if serr := store.Save(path); serr != nil && err == nil {
			err = serr
		}
	}
	return err
}

// modelsReport writes the models a provider offers with their score and
// price, best first, and marks the two automatic picks. available is the
// provider's own list, used for the models the catalogue does not know.
func modelsReport(w io.Writer, cfg *config.Config, info llm.Info, available []string, filter string) {
	fast, smart := tiers(cfg, info)
	match := func(name string) bool {
		return filter == "" || strings.Contains(strings.ToLower(name), strings.ToLower(filter))
	}
	mark := func(name string) string {
		switch {
		case name == fast && name == smart:
			return i18n.T("models.mark_both")
		case name == fast:
			return i18n.T("models.mark_fast")
		case name == smart:
			return i18n.T("models.mark_smart")
		}
		return ""
	}
	listed := map[string]bool{}
	estimated := false
	if modelStore != nil && len(modelStore.Models) > 0 && cfg.Catalog != config.CatalogOff {
		fmt.Fprintf(w, "  %-40s %7s %9s %9s\n", i18n.T("models.col_model"), i18n.T("models.col_score"), i18n.T("models.col_in"), i18n.T("models.col_out"))
		for _, c := range modelStore.For(info.Name, available, time.Now()) {
			listed[c.Native] = true
			if !match(c.Native) {
				continue
			}
			score := "—"
			if c.Score > 0 {
				score = fmt.Sprintf("%.1f", c.Score)
				if c.Estimated {
					score, estimated = "~"+score, true
				}
			}
			lead := "  "
			if mark(c.Native) != "" {
				lead = "▸ "
			}
			fmt.Fprintf(w, "%s%-40s %7s %9s %9s  %s\n", lead, c.Native, score, dollars(c.In), dollars(c.Out), mark(c.Native))
		}
	}
	sort.Strings(available)
	for _, m := range available {
		if listed[m] || !match(m) || strings.HasSuffix(m, ":batch") {
			continue
		}
		lead := "  "
		if mark(m) != "" {
			lead = "▸ "
		}
		fmt.Fprintf(w, "%s%-40s %s  %s\n", lead, m, priceBadge(info.Name, m), mark(m))
	}
	fmt.Fprintln(w)
	if estimated {
		fmt.Fprintln(w, i18n.T("models.estimated"))
	}
	switch {
	case info.Local:
		// A local server has no price list to compare.
	case cfg.Catalog == config.CatalogOff:
		fmt.Fprintln(w, i18n.T("models.off"))
	case modelStore == nil || len(modelStore.Models) == 0:
		fmt.Fprintln(w, i18n.T("models.none"))
	default:
		fmt.Fprintln(w, i18n.T("models.source", modelStore.Fetched.Local().Format("2006-01-02 15:04")))
	}
	if m := cfg.ModelFor(info.Name); m != "" && m != "auto" {
		fmt.Fprintln(w, i18n.T("models.fixed", m, fast, smart))
	} else if fast != "" {
		fmt.Fprintln(w, i18n.T("models.auto", fast, smart))
		if since := livePicks(cfg, info.Name).Since; !since.IsZero() && cfg.Providers[info.Name].FastModel == "" && cfg.Providers[info.Name].SmartModel == "" {
			fmt.Fprintln(w, i18n.T("models.since", since.Local().Format("2006-01-02")))
		}
	}
}

func dollars(v float64) string {
	if v <= 0 {
		return "—"
	}
	return "$" + strconv.FormatFloat(v, 'f', -1, 64)
}
