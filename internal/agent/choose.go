package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// propose_actions: the cards a person answers, and what a run nobody
// watches takes by itself.

func toolProposeActions(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	question := getString(in, "question")
	multi, _ := in["multiple"].(bool)
	raw, _ := in["options"].([]any)
	var opts []Choice
	labels := map[string]string{}
	recommended := map[string]bool{}
	for _, r := range raw {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		c := Choice{ID: getString(m, "id"), Label: getString(m, "label"), Detail: getString(m, "detail"),
			Impact: getString(m, "impact"), Risk: getString(m, "risk")}
		if c.ID == "" || c.Label == "" {
			continue
		}
		if rec, _ := m["recommended"].(bool); rec {
			recommended[c.ID] = true
		}
		opts = append(opts, c)
		labels[c.ID] = c.Label
	}
	if len(opts) == 0 {
		return "", fmt.Errorf("options is empty")
	}
	if a.opts.Unattended {
		return a.autoChoose(question, opts, multi, recommended, labels), nil
	}
	if !a.opts.Interactive {
		a.logChoice(question, opts, multi, nil, "nobody (non-interactive run)")
		return "No user is available to choose (non-interactive run). Do not carry out these actions; " +
			"list them in your summary instead.", nil
	}
	ids := a.choose(ctx, question, opts, multi)
	// A card for one answer that comes back with several is not an answer:
	// acting on it would carry out choices that exclude each other.
	if !multi && len(ids) > 1 {
		a.logChoice(question, opts, multi, ids, "the user (rejected: several answers to a single-choice question)")
		return "The card returned several answers to a question that takes exactly one, so nothing was chosen. " +
			"Do not carry out any of them. Ask again once with propose_actions if the decision matters; otherwise " +
			"list the options in your summary.", nil
	}
	a.logChoice(question, opts, multi, ids, "the user")
	if len(ids) == 0 {
		return "The user chose none of the options. Do not carry them out.", nil
	}
	var b strings.Builder
	b.WriteString("The user selected:\n")
	for _, id := range ids {
		fmt.Fprintf(&b, "- %s: %s\n", id, labels[id])
	}
	b.WriteString("Carry out exactly these, in order, then summarise. Approval rules still apply to each command.")
	if a.Mode() == ModeReadOnly && a.opts.DryRun {
		b.WriteString("\nThis is a dry run (--dry-run): nothing may change, so do not carry them out; describe what each would do.")
	} else if a.Mode() == ModeReadOnly {
		// The selection lifts read-only mode for what was picked, not for
		// everything: each command that changes something is still put to
		// the user, also after --yes, which was given before the analysis
		// and not for these actions.
		a.mu.Lock()
		a.mode = ModeAsk
		a.mu.Unlock()
		a.emit(Event{Kind: EvMode, Mode: ModeAsk})
		b.WriteString("\nRead-only mode is now lifted so you can perform the selected actions, and only those; " +
			"each command that changes something is shown to the user for approval.")
	}
	return b.String(), nil
}

// noChange recognises an option that means "leave it as it is". A run
// that takes options on its own must never take it together with the
// change it excludes.
var noChange = regexp.MustCompile(`(?i)^(none|nothing|skip|keep|leave|no[-_ ]?change|do[-_ ]?nothing|cancel)\b|^(no|don'?t|do not|leave|keep|skip|nada|no cambiar|dejar|mantener|ne rien|nichts|nenhum)\b`)

// autoTakes reports an option a run nobody watches may take by itself:
// one the model recommended and said, explicitly, only reads or makes a
// routine change. The risk is optional for the model, so an option
// without one is left to the user, like a dangerous one.
func autoTakes(c Choice, recommended bool) bool {
	return recommended && (c.Risk == "read-only" || c.Risk == "normal") && !noChange.MatchString(c.ID) && !noChange.MatchString(c.Label)
}

// autoChoose answers a card in a run nobody watches. It takes only the
// options autoTakes allows; for a question with one answer, only when
// exactly one is. The model is told plainly that nobody chose: earlier runs
// reported the automatic picks as "the ones you chose".
func (a *Agent) autoChoose(question string, opts []Choice, multi bool, recommended map[string]bool, labels map[string]string) string {
	var taken, left []string
	if multi {
		for _, c := range opts {
			if !autoTakes(c, recommended[c.ID]) {
				left = append(left, c.ID)
				continue
			}
			taken = append(taken, c.ID)
		}
	} else {
		var recs []string
		for _, c := range opts {
			if autoTakes(c, recommended[c.ID]) {
				recs = append(recs, c.ID)
			}
		}
		if len(recs) == 1 {
			taken = recs
		}
		for _, c := range opts {
			if len(taken) == 0 || c.ID != taken[0] {
				left = append(left, c.ID)
			}
		}
	}
	a.logChoice(question, opts, multi, taken, "omega, automatically (unattended run: nobody chose)")
	var b strings.Builder
	b.WriteString("UNATTENDED RUN: nobody saw this card and nobody chose. ")
	if len(taken) == 0 {
		b.WriteString("No option was taken.")
	} else {
		b.WriteString("omega took these automatically:\n")
		for _, id := range taken {
			fmt.Fprintf(&b, "- %s: %s\n", id, labels[id])
		}
	}
	if len(left) > 0 {
		b.WriteString("\nNot taken (not recommended, dangerous or without a stated risk, a \"leave it\" choice, or a single-choice question without exactly one such option): ")
		var names []string
		for _, id := range left {
			names = append(names, id+" ("+labels[id]+")")
		}
		b.WriteString(strings.Join(names, "; "))
		b.WriteString(". List them in your summary as suggestions for the user.")
	}
	b.WriteString("\nCarry out only what was taken; actions that always need a person are still skipped. In your summary, never say the user chose or asked for any of this.")
	if len(taken) > 0 && a.Mode() == ModeReadOnly && a.opts.DryRun {
		b.WriteString("\nThis is a dry run (--dry-run): nothing may change, so do not carry them out; describe what each would do.")
	} else if len(taken) > 0 && a.Mode() == ModeReadOnly {
		// The user approved the upkeep in advance when they started the
		// unattended run: what was taken may now be done.
		mode := ModeAsk
		if a.opts.AutoYes {
			mode = ModeAuto
		}
		a.mu.Lock()
		a.mode = mode
		a.mu.Unlock()
		a.emit(Event{Kind: EvMode, Mode: mode})
	}
	return b.String()
}

// logChoice writes the answer to a card in the transcript, with who gave it.
func (a *Agent) logChoice(question string, opts []Choice, multi bool, ids []string, who string) {
	if a.opts.Unattended || !a.opts.Interactive {
		// No card was shown, so the question and the options are not in
		// the transcript yet.
		a.writeLog(Event{Kind: EvChoice, Text: question, Choices: opts, Multi: multi})
	}
	picked := strings.Join(ids, ", ")
	if picked == "" {
		picked = "nothing"
	}
	a.writeLog(Event{Kind: EvAssistant, Text: "? answer: " + picked + " — chosen by " + who})
}
