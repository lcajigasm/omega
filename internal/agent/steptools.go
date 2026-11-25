package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/safety"
	"github.com/lcajigasm/omega/internal/steps"
)

// list_steps and run_step: the catalogue of maintenance steps.

// toolListSteps lists the catalogue. By default only the steps this
// machine can run, without their commands: the whole catalogue with every
// command was some 24 KB, sent again on each call. id gives one step in
// full; all=true adds the ones that are not installed or are disabled.
func toolListSteps(_ context.Context, a *Agent, in map[string]any) (string, error) {
	if id := getString(in, "id"); id != "" {
		s, ok := steps.Find(id)
		if !ok {
			return "", fmt.Errorf("no step %q; call list_steps without id for the list", id)
		}
		return stepDetail(a, s), nil
	}
	cat := getString(in, "category")
	every, _ := in["all"].(bool)
	detail, _ := in["detail"].(bool)
	var b strings.Builder
	hidden := 0
	for _, s := range steps.All {
		if cat != "" && cat != "all" && s.Category != cat {
			continue
		}
		status := stepStatus(a, s)
		if status != "available" && !every {
			hidden++
			continue
		}
		fmt.Fprintf(&b, "- %s [%s, risk %s", s.ID, s.Category, s.Risk)
		if status != "available" {
			b.WriteString(", " + status)
		}
		fmt.Fprintf(&b, "]: %s", s.Name)
		if detail {
			fmt.Fprintf(&b, " — `%s`", s.Command)
		}
		if s.Description != "" {
			b.WriteString(" — " + s.Description)
		}
		b.WriteString("\n")
	}
	if hidden > 0 {
		fmt.Fprintf(&b, "(%d more steps are not installed or are disabled; all=true lists them with how to install them)\n", hidden)
	}
	b.WriteString("For the command a step runs, call list_steps with its id.\n")
	return b.String(), nil
}

// stepStatus says whether a step can run here.
func stepStatus(a *Agent, s steps.Step) string {
	switch {
	case a.opts.Config.Skipped(s.ID):
		return "disabled by the user"
	case !a.opts.SkipDetect && !s.Available():
		status := "not installed"
		if s.Install != "" {
			status += "; install with: " + s.Install
		}
		return status
	}
	return "available"
}

// stepDetail is everything list_steps knows of one step.
func stepDetail(a *Agent, s steps.Step) string {
	var b strings.Builder
	fmt.Fprintf(&b, "id: %s\nname: %s\ncategory: %s\nrisk: %s\nstatus: %s\ncommand: %s\n", s.ID, s.Name, s.Category, s.Risk, stepStatus(a, s), s.Command)
	if s.AlwaysConfirm || safety.AlwaysConfirm(s.Command) {
		b.WriteString("asked every time: yes, also under --yes; skipped in unattended runs\n")
	}
	if s.Description != "" {
		b.WriteString("about: " + s.Description + "\n")
	}
	return b.String()
}

func toolRunStep(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	id := getString(in, "id")
	s, ok := steps.Find(id)
	if !ok {
		return "", fmt.Errorf("no step %q; use list_steps", id)
	}
	if a.opts.Config.Skipped(id) {
		return "", fmt.Errorf("the user disabled step %q in the configuration", id)
	}
	if !a.opts.SkipDetect && !s.Available() {
		return "", fmt.Errorf("step %q is not available on this machine", id)
	}
	if why, off := notUpkeepSteps[id]; off && a.opts.Unattended {
		return "", a.notUpkeep("run_step:"+id, s.Command, "the "+id+" step was not run: "+why)
	}
	risk := s.Risk
	if safety.Classify(s.Command) == safety.Blocked {
		risk = safety.Blocked
	}
	reason := i18n.TOr("step."+id, s.Name)
	if s.Description != "" {
		reason += " · " + s.Description
	}
	again, _ := in["again"].(bool)
	if since, ok := lastClean(id); ok && !again {
		msg := fmt.Sprintf("%s ran successfully %s ago, in this or an earlier run; running it again now would free little, so it was "+
			"not run. Say so in the summary. Run it again only if the user asks (again=true).", id, since.Round(time.Minute))
		a.writeLog(Event{Kind: EvAssistant, Text: "[run_step:" + id + " skipped: " + msg + "]"})
		return msg, nil
	}
	// The catalogue's flag, or what the policy says of the command itself:
	// mo purge --yes deletes build folders whatever the step declares.
	always := s.AlwaysConfirm || safety.AlwaysConfirm(s.Command)
	out, err := a.runShell(ctx, "run_step:"+id, s.Command, reason, risk, always)
	if err == nil && strings.HasPrefix(out, "exit_code: 0\n") {
		markClean(id)
	}
	return out, err
}

// cooldown are the cleaners that are not worth running twice in a few
// hours: one Mac ran mo clean four times in 25 minutes, 40 to 75 seconds
// each, for a few megabytes after the first.
var cooldown = map[string]time.Duration{"mole-clean": 6 * time.Hour, "mole-purge": 6 * time.Hour}

func cleanStamp(id string) string { return filepath.Join(config.StateDir(), "last-"+id) }

// lastClean says how long ago a cooled-down step last succeeded, when that
// is within its cooldown.
func lastClean(id string) (time.Duration, bool) {
	d, ok := cooldown[id]
	if !ok {
		return 0, false
	}
	fi, err := os.Stat(cleanStamp(id))
	if err != nil {
		return 0, false
	}
	since := time.Since(fi.ModTime())
	return since, since < d
}

func markClean(id string) {
	if _, ok := cooldown[id]; !ok {
		return
	}
	if os.MkdirAll(config.StateDir(), 0o700) == nil {
		os.WriteFile(cleanStamp(id), nil, 0o600)
		now := time.Now()
		os.Chtimes(cleanStamp(id), now, now)
	}
}
