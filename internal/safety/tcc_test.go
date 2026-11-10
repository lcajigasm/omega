package safety

import "testing"

// Reading the privacy database is how omega finds stale permissions; only
// writing it is refused. omega's own operation deletes a single row after
// tccutil fails, and is not a shell command the model can compose.
func TestPrivacyDatabaseIsReadOnlyForTheModel(t *testing.T) {
	read := `sqlite3 -readonly "$HOME/Library/Application Support/com.apple.TCC/TCC.db" "select service, client from access"`
	if Classify(read) == Blocked {
		t.Errorf("reading the database must be allowed: %s", read)
	}
	if Classify("/usr/local/bin/omega mac op perm-reset 'Accessibility' '/gone/node'") == Blocked {
		t.Error("omega's own permission reset must not be blocked")
	}
}
