package cli

import (
	"os"
	"testing"
)

// The tests never download the model catalogue: what they check must not
// depend on which models are on sale today.
func TestMain(m *testing.M) {
	catalogDisabled = true
	os.Exit(m.Run())
}
