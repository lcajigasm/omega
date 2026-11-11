package macos

import "testing"

func TestBlockedAccess(t *testing.T) {
	cases := []struct {
		out, want string
	}{
		{"ls: /Users/luis/Library/Mail: Operation not permitted", "SystemPolicyAllFiles"},
		{"Error: unable to open database \"/Users/luis/Library/Application Support/com.apple.TCC/TCC.db\": authorization denied", "SystemPolicyAllFiles"},
		{"NOTE: the user's privacy database could not be read, so only", "SystemPolicyAllFiles"},
		{"tmutil: listbackups requires Full Disk Access privileges.", "SystemPolicyAllFiles"},
		{"find: /Users/ana/Documents/taxes: Operation not permitted", "SystemPolicyDocumentsFolder"},
		{"du: /Users/ana/Downloads: Operation not permitted", "SystemPolicyDownloadsFolder"},
		{"35:62: execution error: Not authorized to send Apple events to System Events. (-1743)", "AppleEvents"},
		{"osascript is not allowed assistive access. (-1719)", "Accessibility"},
		// Not privacy protection: ownership, or System Integrity Protection.
		{"rm: /System/Library/CoreServices/Finder.app: Operation not permitted", ""},
		{"chown: /usr/local/bin/x: Operation not permitted", ""},
		{"rm: /Users/ana/Dev/project/node_modules: Permission denied", ""},
		{"exit_code: 0\n---\nall good", ""},
	}
	for _, c := range cases {
		got, ok := BlockedAccess(c.out)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("BlockedAccess(%q) = %q, %v; want %q", c.out, got, ok, c.want)
		}
	}
}

func TestAsksItself(t *testing.T) {
	if AsksItself("SystemPolicyAllFiles") || AsksItself("kTCCServiceAccessibility") {
		t.Error("Full Disk Access and Accessibility have no dialog: only System Settings")
	}
	if !AsksItself("kTCCServiceAppleEvents") || !AsksItself("SystemPolicyDocumentsFolder") {
		t.Error("Automation and Documents are asked by macOS itself")
	}
}

func TestForgetAnswerRefusesOddBundles(t *testing.T) {
	if err := ForgetAnswer("AppleEvents", "x; rm -rf ~"); err == nil {
		t.Error("a bundle id that is not one was accepted")
	}
}
