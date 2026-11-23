package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/pending"
	"github.com/lcajigasm/omega/internal/safety"
)

func pendingToolSpec() llm.ToolSpec {
	return llm.ToolSpec{Name: "pending", Description: "What earlier runs left for the user to do by hand, kept between runs so it is " +
		"not rediscovered and listed again every time. list shows the items with their status. add records an item left for the " +
		"user at the end of this run (reuse the id of the same item; new items get a short id of a few words). resolve marks an " +
		"item done when a measurement in THIS session shows it is gone. Only the user dismisses or postpones items (omega pending).",
		Properties: map[string]any{
			"action": map[string]any{"type": "string", "enum": []string{"list", "add", "resolve"}},
			"id":     str("item id, e.g. time-machine, docker-build-cache, wt-1806-dame-datos"),
			"text":   str("for add: what the user has to do and where, one or two sentences"),
			"area":   str("for add: optional area, e.g. backup, disk, docker, login"),
		}, Required: []string{"action"}}
}

func toolPending(_ context.Context, a *Agent, in map[string]any) (string, error) {
	action := getString(in, "action")
	start := time.Now()
	cmd := strings.TrimSpace("pending " + action + " " + getString(in, "id"))
	a.emit(Event{Kind: EvToolStart, Tool: "pending", Command: cmd, Risk: safety.Safe})
	out, err := pendingAction(action, getString(in, "id"), getString(in, "text"), getString(in, "area"), time.Now())
	a.quiet("pending", cmd, start, out, err)
	if err != nil {
		return "", explained(err)
	}
	return out, nil
}

func pendingAction(action, id, text, area string, now time.Time) (string, error) {
	items, err := pending.Load()
	if err != nil {
		return "", err
	}
	switch action {
	case "", "list":
		if len(items) == 0 {
			return "No pending items are recorded.", nil
		}
		var b strings.Builder
		for _, it := range items {
			fmt.Fprintf(&b, "- %s [%s] %s (since %s, seen in %d runs)\n", it.ID, it.Status, it.Text, it.FirstSeen.Format("2006-01-02"), it.Seen)
		}
		return b.String(), nil
	case "add":
		items, changed, err := pending.Note(items, id, text, area, now)
		if err != nil {
			return "", err
		}
		if err := pending.Save(items); err != nil {
			return "", err
		}
		for _, it := range items {
			if it.ID == pending.CleanID(id) {
				switch {
				case it.Status == pending.Dismissed:
					return "recorded; the user dismissed this item: do not mention it in the summary", nil
				case it.Status == pending.Snoozed && !it.Active(now):
					return "recorded; the user postponed this item until " + it.Until.Format("2006-01-02") + ": do not mention it", nil
				case changed:
					return "recorded as new or changed: mention it in full in the summary", nil
				}
				return fmt.Sprintf("recorded; already known since %s (seen in %d runs): count it in the one-line mention of known items", it.FirstSeen.Format("2006-01-02"), it.Seen), nil
			}
		}
		return "recorded", nil
	case "resolve":
		items, err := pending.Set(items, pending.CleanID(id), pending.Done, time.Time{})
		if err != nil {
			return "", err
		}
		return "marked done", pending.Save(items)
	}
	return "", errors.New("action is list, add or resolve")
}

// pendingPrompt is what a run is told about the items it knows.
func pendingPrompt() string {
	items, err := pending.Load()
	if err != nil {
		return ""
	}
	s := pending.Summary(items, time.Now())
	if s == "" {
		return ""
	}
	return "\n" + s + "Re-check an item before you report it, record what is still left with pending add (same id) and what is gone " +
		"with pending resolve. In the summary, give new or changed items in full and the rest as one line: \"N known pending items " +
		"(see omega pending)\".\n"
}
