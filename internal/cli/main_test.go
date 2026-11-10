package cli

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
)

// The tests never download the model catalogue or ask GitHub for the latest
// release: what they check must not depend on what is on sale or published today.
func TestMain(m *testing.M) {
	catalogDisabled = true
	latestRelease = func() string { return "" }
	cobra.EnablePrefixMatching = true // as Execute sets it
	os.Exit(m.Run())
}
