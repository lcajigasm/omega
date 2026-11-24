package agent

import (
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/i18n"
)

// E6 of the fifth review: the cost limit is said in the user's language.
func TestCostLimitIsTranslated(t *testing.T) {
	t.Cleanup(func() { i18n.Init("en") })
	i18n.Init("es")
	if got := i18n.T("agent.cost_limit", 1.5); !strings.Contains(got, "1.50") || strings.Contains(got, "Reached") {
		t.Fatalf("got %q", got)
	}
}
