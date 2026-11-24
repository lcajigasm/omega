package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/safety"
	"github.com/lcajigasm/omega/internal/steps"
	"github.com/lcajigasm/omega/internal/ui"
)

// stepDetail is one step as `omega steps show --json` gives it.
type stepDetail struct {
	StepInfo
	Description   string `json:"description,omitempty"`
	Requires      string `json:"requires,omitempty"`
	Install       string `json:"install,omitempty"`
	AlwaysConfirm bool   `json:"always_confirm"`
	Builtin       bool   `json:"builtin"`
	// LastRuns are the durations `omega custom` recorded, newest first.
	LastRuns []customRecord `json:"last_runs,omitempty"`
}

func stepsShowCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:               "show <id>",
		Short:             i18n.T("cmd.steps_show"),
		Args:              cobra.ExactArgs(1),
		Example:           "  omega steps show brew\n  omega steps show mole-purge --json",
		ValidArgsFunction: completeAllSteps,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			s, ok := steps.Find(args[0])
			if !ok {
				return usageErr(cmd, i18n.T("steps.unknown", args[0]))
			}
			d := detailOf(cfg, s)
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), d)
			}
			writeStepDetail(cmd.OutOrStdout(), d)
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.json"))
	return c
}

func detailOf(cfg *config.Config, s steps.Step) stepDetail {
	d := stepDetail{
		StepInfo: StepInfo{ID: s.ID, Name: i18n.TOr("step."+s.ID, s.Name), Category: i18n.T("cat." + s.Category),
			Risk: ui.RiskName(s.Risk), Dangerous: s.Risk >= safety.Dangerous, Installed: s.Available(), Skipped: cfg.Skipped(s.ID),
			CategoryID: s.Category, RiskID: s.Risk.String(), Command: s.Command, Available: s.Available()},
		Description: s.Description, Requires: s.Requires, Install: s.Install,
		AlwaysConfirm: s.AlwaysConfirm || safety.AlwaysConfirm(s.Command), Builtin: steps.Builtin(s.ID),
	}
	d.LastRuns = loadCustomHistory().runs(s.ID)
	return d
}

// writeStepDetail is everything omega knows about one step.
func writeStepDetail(w io.Writer, d stepDetail) {
	r := ui.NewReport(w)
	r.Title("omega steps show", d.ID)
	r.Blank()
	label := 14
	r.Check(ui.MarkNone, i18n.T("steps.f_name"), label, r.Bold(d.Name))
	r.Check(ui.MarkNone, i18n.T("steps.f_category"), label, d.Category)
	risk := r.StepRisk(d.RiskID)
	if d.AlwaysConfirm {
		risk += r.Muted(" · " + i18n.T("steps.always_asks"))
	}
	r.Check(ui.MarkNone, i18n.T("steps.f_risk"), label, risk)
	switch {
	case d.Skipped:
		r.Check(ui.MarkSkip, i18n.T("steps.f_state"), label, i18n.T("doctor.skipped"))
	case d.Installed:
		r.Check(ui.MarkOK, i18n.T("steps.f_state"), label, i18n.T("steps.installed"))
	default:
		r.Check(ui.MarkOff, i18n.T("steps.f_state"), label, i18n.T("steps.missing", d.Requires))
	}
	if !d.Installed && d.Install != "" {
		r.Check(ui.MarkNone, i18n.T("steps.f_install"), label, d.Install)
	}
	if !d.Builtin {
		r.Check(ui.MarkNone, i18n.T("steps.f_source"), label, i18n.T("steps.own"))
	}
	if d.Description != "" {
		r.Section(i18n.T("steps.f_what"), -1)
		r.Detail(d.Description)
	}
	r.Section(i18n.T("steps.f_command"), -1)
	for _, l := range strings.Split(d.Command, "\n") {
		fmt.Fprintln(w, "  "+l)
	}
	if len(d.LastRuns) > 0 {
		r.Section(i18n.T("steps.f_last"), -1)
		for _, run := range d.LastRuns {
			mark := ui.MarkOK
			if !run.OK {
				mark = ui.MarkErr
			}
			r.Check(mark, run.When.Local().Format("2006-01-02 15:04"), 16, ui.Duration(run.Took))
		}
	}
	r.Blank()
	r.Hint(i18n.T("steps.show_hint", d.ID))
}
