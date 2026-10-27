package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/ui"
)

// modelPlan is the model choice for one run.
type modelPlan struct {
	Model    string
	Escalate string // switch to this model when a step fails; "" for none
}

// tiers returns the fast and smart models for a provider, honouring the
// user's overrides.
func tiers(cfg *config.Config, info llm.Info) (fast, smart string) {
	pc := cfg.Providers[info.Name]
	fast, smart = info.FastModel, info.SmartModel
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
	return fmt.Sprintf("$%g / $%g per M", p.Input, p.Output)
}

// chooseModel shows the model card for a provider and saves the choice.
func chooseModel(cfg *config.Config, info llm.Info, p llm.Provider) error {
	fast, smart := tiers(cfg, info)
	var opts []ui.Option
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
	models, err := p.Models(ctx)
	cancel()
	if err != nil {
		fmt.Println(i18n.T("setup.models_failed", err))
	}
	sort.Strings(models)
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
	if len(opts) == 0 {
		return fmt.Errorf("%s", i18n.T("model.need_local", info.Name))
	}
	tty := term.IsTerminal(int(os.Stdin.Fd()))
	ids, err := ui.Pick(i18n.T("model.title", info.Label), i18n.T("model.hint"), opts, false, tty, os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	choice := ids[0]
	pc := cfg.Providers[info.Name]
	if choice == "auto" {
		pc.Model = ""
	} else {
		pc.Model = choice
	}
	cfg.Providers[info.Name] = pc
	if cfg.Provider == info.Name {
		cfg.Model = pc.Model
	}
	if err := cfg.Save(); err != nil {
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
