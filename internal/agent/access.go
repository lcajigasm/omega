package agent

import (
	"context"
	"fmt"

	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/macos"
)

// When macOS privacy protection stops a tool, omega does not just report it:
// it shows a card that names the permission and the app that needs it, opens
// the place where it is granted (macOS's own dialog when the permission has
// one, the pane of System Settings otherwise), waits for the user and runs
// the tool again. Each permission is asked once per session.

// Seams the tests replace.
var (
	accessHost   = macos.HostApp
	accessOpen   = macos.OpenPrivacyPane
	accessForget = macos.ForgetAnswer
)

// accessOutcome is what happened when a permission was asked for.
type accessOutcome int

const (
	accessNobody   accessOutcome = iota // nobody is there to grant it
	accessGranted                       // the user says it is on now
	accessDeclined                      // the user goes on without it
	accessRepeat                        // already asked in this session
)

// blockedAccess reports the privacy permission that stopped a tool, read
// from what the tool returned.
func blockedAccess(out string, err error) (string, bool) {
	if goos != "darwin" {
		return "", false
	}
	if err != nil {
		out += "\n" + err.Error()
	}
	return macos.BlockedAccess(out)
}

// askAccess shows the cards that get a permission granted. It serializes
// with other tools of the same batch, so two cards never race.
func (a *Agent) askAccess(ctx context.Context, service string) accessOutcome {
	a.accessMu.Lock()
	defer a.accessMu.Unlock()
	if a.accessAsked[service] {
		return accessRepeat
	}
	if a.accessAsked == nil {
		a.accessAsked = map[string]bool{}
	}
	a.accessAsked[service] = true
	if !a.opts.Interactive {
		return accessNobody
	}
	bundle, app := accessHost()
	if app == "" {
		app = "omega"
	}
	perm := macos.ServiceName(service)
	opts := []Choice{}
	if macos.AsksItself(service) && bundle != "" {
		opts = append(opts, Choice{ID: "dialog", Label: i18n.T("access.dialog"), Detail: i18n.T("access.dialog_detail", app), Risk: "normal"})
	}
	opts = append(opts,
		Choice{ID: "settings", Label: i18n.T("access.settings"), Detail: i18n.T("access.settings_detail", perm, app), Risk: "read-only"},
		Choice{ID: "skip", Label: i18n.T("access.skip"), Detail: i18n.T("access.skip_detail"), Risk: "read-only"})
	picked := a.choose(ctx, i18n.T("access.ask", app, perm), opts, false)
	if len(picked) == 0 {
		return accessDeclined
	}
	switch picked[0] {
	case "dialog":
		// macOS asks again by itself when the tool is retried.
		if accessForget(service, bundle) == nil {
			return accessGranted
		}
		fallthrough
	case "settings":
		if err := accessOpen(service); err != nil {
			a.emit(Event{Kind: EvError, Text: err.Error()})
		}
		again := a.choose(ctx, i18n.T("access.wait", perm, app), []Choice{
			{ID: "retry", Label: i18n.T("access.retry"), Risk: "read-only"},
			{ID: "skip", Label: i18n.T("access.skip"), Risk: "read-only"},
		}, false)
		if len(again) == 1 && again[0] == "retry" {
			return accessGranted
		}
	}
	return accessDeclined
}

// regainAccess handles a tool that macOS privacy protection stopped: it gets
// the permission granted when someone is there, runs the tool again when
// that is safe, and tells the model how it went.
func (a *Agent) regainAccess(ctx context.Context, call llm.ToolCall, service string, rerun func() (string, error), out string, err error) (string, error) {
	perm := macos.ServiceName(service)
	_, app := accessHost()
	if app == "" {
		app = "the app that runs omega"
	}
	note := ""
	switch a.askAccess(ctx, service) {
	case accessNobody:
		note = fmt.Sprintf("macOS blocked this: %s lacks the %q permission, and nobody is here to grant it. Say in the summary what could not be done "+
			"and that the user can grant it with `%s` and run omega again.", app, perm, macos.PrivacyPane(service))
	case accessDeclined:
		note = fmt.Sprintf("macOS blocked this for lack of the %q permission, and the user chose to go on without it. Do not ask again; "+
			"carry on with the rest and say in the summary what could not be checked.", perm)
	case accessRepeat:
		note = fmt.Sprintf("macOS still blocks this for lack of the %q permission; the user was already asked in this session. Carry on with the rest.", perm)
	case accessGranted:
		if !readOnlyCall(call) && call.Name != "mac_inventory" {
			note = fmt.Sprintf("macOS blocked part of this for lack of the %q permission. The user has just granted it: run it again if its result still matters.", perm)
			break
		}
		a.macInv = nil
		out, err = rerun()
		if _, still := blockedAccess(out, err); still {
			note = fmt.Sprintf("macOS still blocks this after the user turned on %q: the permission takes effect once %s is quit and opened again. "+
				"Tell the user to reopen it and run omega again; do not retry now.", perm, app)
		}
	}
	if note == "" {
		return out, err
	}
	if err != nil {
		return out, fmt.Errorf("%w\n\nNOTE: %s", err, note)
	}
	return out + "\n\nNOTE: " + note, nil
}
