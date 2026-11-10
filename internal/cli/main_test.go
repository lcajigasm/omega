package cli

import (
	"os"
	"testing"
)

// The tests never download the model catalogue or ask GitHub for the latest
// release: what they check must not depend on what is on sale or published today.
func TestMain(m *testing.M) {
	catalogDisabled = true
	latestRelease = func() string { return "" }
	os.Exit(m.Run())
}
