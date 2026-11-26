package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/ui"
)

// omega ultra is the whole of omega in one attended run: every stage of
// omega full, plus a deep clean of what the cleaning steps never reach and a
// pass through every review omega has (AI agents, configuration assistants,
// security, privacy, network, skills). Its sessions run on stronger models,
// with more effort and more turns, dig deeper, must show the evidence for
// every change they offer, and put every change to the person.

// ultraTurns is how many turns an ultra stage may take: it measures,
// inspects and checks again far more than an ordinary one.
const ultraTurns = 200

func init() { agent.OmegaRun = runOmega }

// ultraStages are omega full's stages with the two only ultra has: the
// deep clean after Docker, the reviews after tune.
func ultraStages() []fullStage {
	deep := fullStage{id: "deep", command: "omega ultra --only deep", task: agent.TaskUltraDeep, risk: "dangerous"}
	care := fullStage{id: "care", command: "omega ultra --only care", task: agent.TaskUltraCare, risk: "normal"}
	var out []fullStage
	for _, s := range fullStages {
		out = append(out, s)
		switch s.id {
		case "docker":
			out = append(out, deep)
		case "tune":
			out = append(out, care)
		}
	}
	return out
}

// ultraTier is the provider's most capable model: the user's ultra_model,
// else the provider's, else the smart one.
func ultraTier(cfg *config.Config, info llm.Info) string {
	_, smart := tiers(cfg, info)
	if m := cfg.Providers[info.Name].UltraModel; m != "" {
		return m
	}
	if info.UltraModel != "" {
		return info.UltraModel
	}
	return smart
}

// ultraPlan decides the models of an ultra stage. A model given with
// --model is used as it is. Otherwise the stages start on the smart tier,
// never the fast one, and move to the ultra tier when a step fails; the
// deep clean, where the analysis is hardest and a mistake costs most,
// starts on the ultra tier.
func ultraPlan(cfg *config.Config, info llm.Info, flagModel, task string) modelPlan {
	fast, smart := tiers(cfg, info)
	top := ultraTier(cfg, info)
	var plan modelPlan
	switch {
	case flagModel != "":
		plan = modelPlan{Model: flagModel}
	case task == agent.TaskUltraDeep:
		plan = modelPlan{Model: top}
	case smart != top:
		plan = modelPlan{Model: smart, Escalate: top}
	default:
		plan = modelPlan{Model: smart}
	}
	plan.Choices = uniqueModels(plan.Model, plan.Escalate, top, smart, fast)
	return plan
}

// efforts in order: ultra never lowers what the user configured.
var efforts = []string{"low", "medium", "high", "xhigh", "max"}

// ultraEffort is the effort of an ultra stage: high, or the configured one
// when it is higher.
func ultraEffort(configured string) string {
	rank := func(e string) int {
		for i, v := range efforts {
			if v == e {
				return i
			}
		}
		return -1
	}
	if rank(configured) > rank("high") {
		return configured
	}
	return "high"
}

func ultraCmd() *cobra.Command {
	var f runFlags
	var skip, only []string
	c := &cobra.Command{
		Use:     "ultra",
		Aliases: []string{"deep"},
		Short:   i18n.T("cmd.ultra"),
		Long:    i18n.T("cmd.ultra_long"),
		Example: "  omega ultra\n  omega ultra --only deep\n  omega ultra --skip update,mac\n  omega ultra --dry-run",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := f.validate(); err != nil {
				return err
			}
			// Ultra is the run where a person confirms everything: the
			// flags that answer for them have no place in it.
			for _, fl := range []struct {
				on   bool
				name string
			}{{f.yes, "--yes"}, {f.auto, "--auto"}, {f.noAI, "--no-ai"}, {f.offline, "--offline"}, {f.wantsJSON(), "--json"}} {
				if fl.on {
					return usageErr(cmd, i18n.T("ultra.flag_refused", fl.name))
				}
			}
			if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
				return fmt.Errorf("%s", i18n.T("ultra.need_tty"))
			}
			here := ultraStagesHere()
			chosen, err := pickUltraStages(here, only, skip)
			if err != nil {
				return err
			}
			byID := map[string]fullStage{}
			var stages []ui.GuideStage
			for _, s := range here {
				byID[s.id] = s
				stages = append(stages, guideStage(s, chosen[s.id]))
			}
			f.stage, f.ultra = true, true
			f.share = newStageShare()
			if f.sudo && goos != "windows" && !f.dryRun {
				if sudoAsked, err = askSudoOnce(); err != nil {
					return err
				}
				defer func() { sudoAsked = "" }()
			}
			info := ui.GuideInfo{Version: version, Ultra: true, Model: ultraModelLabel(f)}
			if f.dryRun {
				info.Flags = append(info.Flags, i18n.T("full.flag_dry"))
			}
			if f.demo {
				info.Flags = append(info.Flags, i18n.T("full.flag_demo"))
			}
			if ui.WantTUI(f.noTUI) {
				res, ok, err := ui.RunGuide(info, stages, func(id string, out io.Writer) error { return runStage(byID[id], f, out) })
				if err != nil || !ok {
					return err
				}
				fullSummary(os.Stdout, stages, res)
				return nil
			}
			// Line by line, still attended: between stages the user decides.
			in := bufio.NewReader(os.Stdin)
			ask := func() string {
				line, err := in.ReadString('\n')
				if err != nil {
					return "q"
				}
				return line
			}
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, os.Interrupt)
			defer signal.Stop(sig)
			stopped := func() bool {
				select {
				case <-sig:
					return true
				default:
					return false
				}
			}
			fmt.Fprintln(os.Stdout, i18n.T("ultra.plan_intro"))
			res := runFull(os.Stdout, stages, func(id string) error { return runStage(byID[id], f, os.Stdout) }, ask, stopped)
			fullSummary(os.Stdout, stages, res)
			return nil
		},
	}
	bindRunFlags(c.Flags(), &f)
	for _, name := range []string{"prompt", "once", "yes", "auto"} {
		c.Flags().MarkHidden(name)
	}
	c.Flags().StringSliceVar(&skip, "skip", nil, i18n.T("flag.full_skip", ultraStageIDList()))
	c.Flags().StringSliceVar(&only, "only", nil, i18n.T("flag.full_only"))
	return c
}

// ultraStagesHere are the ultra stages that apply to this machine.
func ultraStagesHere() []fullStage {
	var out []fullStage
	for _, s := range ultraStages() {
		if s.when == nil || s.when() {
			out = append(out, s)
		}
	}
	return out
}

func ultraStageIDList() string {
	var ids []string
	for _, s := range ultraStages() {
		ids = append(ids, s.id)
	}
	return strings.Join(ids, ", ")
}

// pickUltraStages reads --only and --skip against the ultra stages.
func pickUltraStages(stages []fullStage, only, skip []string) (map[string]bool, error) {
	known := map[string]bool{}
	for _, s := range ultraStages() {
		known[s.id] = true
	}
	split := func(list []string) (map[string]bool, error) {
		out := map[string]bool{}
		for _, item := range list {
			for _, id := range strings.Split(item, ",") {
				if id = strings.ToLower(strings.TrimSpace(id)); id == "" {
					continue
				}
				if !known[id] {
					return nil, fmt.Errorf("%s", i18n.T("full.unknown_stage", id, ultraStageIDList()))
				}
				out[id] = true
			}
		}
		return out, nil
	}
	o, err := split(only)
	if err != nil {
		return nil, err
	}
	sk, err := split(skip)
	if err != nil {
		return nil, err
	}
	chosen := map[string]bool{}
	for _, s := range stages {
		chosen[s.id] = (len(o) == 0 || o[s.id]) && !sk[s.id]
	}
	return chosen, nil
}

// ultraModelLabel is what the top bar says about the models before the
// first stage starts: "smart → ultra".
func ultraModelLabel(f runFlags) string {
	if f.demo {
		return ""
	}
	cfg, err := config.Load()
	if err != nil {
		return ""
	}
	name := f.provider
	if name == "" {
		name = cfg.Provider
	}
	info, ok := llm.Lookup(name)
	if !ok {
		return ""
	}
	if f.model != "" {
		return f.model
	}
	_, smart := tiers(cfg, info)
	if top := ultraTier(cfg, info); top != smart {
		return smart + " → " + top
	}
	return smart
}
