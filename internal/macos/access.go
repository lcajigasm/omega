package macos

import (
	"os"
	posix "path"
	"regexp"
	"strings"
)

// When macOS privacy protection (TCC) stops a command, the command only says
// "Operation not permitted". This file recognises those failures, names the
// permission that is missing and the app that needs it, and opens the place
// where the user can grant it: macOS's own dialog when it has one, the pane
// of System Settings when it does not.

var accessMarks = []struct {
	re      *regexp.Regexp
	service string
}{
	{regexp.MustCompile(`(?i)not authori[sz]ed to send apple events|\(-1743\)`), "AppleEvents"},
	{regexp.MustCompile(`(?i)not allowed assistive access|\(-1719\)|\(-25211\)`), "Accessibility"},
	{regexp.MustCompile(`(?i)requires full disk access|privacy database could not be read|` +
		`TCC\.db.*(authorization denied|unable to open|not permitted)|(authorization denied|unable to open database).*TCC\.db`), "SystemPolicyAllFiles"},
}

// notPermitted is what a denied file access prints.
var notPermitted = regexp.MustCompile(`(?i)operation not permitted`)

// Folders of the home that macOS protects, with the permission of each. The
// ones under Library have no dialog of their own: only Full Disk Access
// opens them.
var protectedFolders = []struct {
	re      *regexp.Regexp
	service string
}{
	{regexp.MustCompile(`/Users/[^/\s]+/Desktop(/|\b)`), "SystemPolicyDesktopFolder"},
	{regexp.MustCompile(`/Users/[^/\s]+/Documents(/|\b)`), "SystemPolicyDocumentsFolder"},
	{regexp.MustCompile(`/Users/[^/\s]+/Downloads(/|\b)`), "SystemPolicyDownloadsFolder"},
	{regexp.MustCompile(`/Users/[^/\s]+/(\.Trash|Library/(Mail|Messages|Safari|Cookies|Calendars|Reminders|Containers|Group Containers|` +
		`Metadata|Suggestions|HomeKit|IdentityServices|Biome|Sharing|Assistant|DoNotDisturb|Autosave Information|` +
		`Application Support/(com\.apple\.TCC|AddressBook|CallHistoryDB|CallHistoryTransactions|MobileSync|Knowledge|FileProvider)))(/|\b)`), "SystemPolicyAllFiles"},
	{regexp.MustCompile(`/Library/Application Support/com\.apple\.TCC(/|\b)`), "SystemPolicyAllFiles"},
}

// BlockedAccess looks at what a command printed and reports the privacy
// permission whose absence stopped it. A plain "Operation not permitted"
// counts only on a folder macOS protects: elsewhere it is file ownership or
// System Integrity Protection, which no permission lifts.
func BlockedAccess(output string) (service string, ok bool) {
	for _, m := range accessMarks {
		if m.re.MatchString(output) {
			return m.service, true
		}
	}
	for _, line := range strings.Split(output, "\n") {
		if !notPermitted.MatchString(line) {
			continue
		}
		for _, f := range protectedFolders {
			if f.re.MatchString(line) {
				return f.service, true
			}
		}
	}
	return "", false
}

// AsksItself reports whether macOS shows its own dialog for a permission the
// first time an app needs it. Forgetting an earlier "Don't Allow" makes the
// dialog come back; the rest can only be turned on in System Settings.
func AsksItself(service string) bool {
	switch ShortService(service) {
	case "AppleEvents", "SystemPolicyDesktopFolder", "SystemPolicyDocumentsFolder", "SystemPolicyDownloadsFolder",
		"SystemPolicyNetworkVolumes", "SystemPolicyRemovableVolumes", "Camera", "Microphone":
		return true
	}
	return false
}

var bundleID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.\-]*$`)

// HostApp is the app macOS holds responsible for what omega does, the one a
// permission has to be given to: the terminal omega runs in, or omega's own
// window. bundle is empty when it cannot be told.
func HostApp() (bundle, name string) {
	if b := os.Getenv("__CFBundleIdentifier"); bundleID.MatchString(b) {
		bundle = b
		if out, err := Output("mdfind", "kMDItemCFBundleIdentifier == '"+b+"'"); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.HasSuffix(line, ".app") {
					name = strings.TrimSuffix(posix.Base(line), ".app")
					break
				}
			}
		}
	}
	if name == "" {
		name = strings.TrimSuffix(os.Getenv("TERM_PROGRAM"), ".app")
		if name == "Apple_Terminal" {
			name = "Terminal"
		}
	}
	if name == "" {
		name = bundle
	}
	return bundle, name
}

// OpenPrivacyPane opens System Settings on the list of a permission.
func OpenPrivacyPane(service string) error {
	_, err := Output("/bin/sh", "-c", PrivacyPane(service))
	return err
}

// ForgetAnswer makes macOS forget what the user answered when bundle asked
// for a permission, so its dialog is shown again the next time.
func ForgetAnswer(service, bundle string) error {
	if !bundleID.MatchString(bundle) {
		return os.ErrInvalid
	}
	_, err := Output("tccutil", "reset", ShortService(service), bundle)
	return err
}
