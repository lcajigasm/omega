package cli

import (
	"fmt"
	"strings"
	"testing"
)

// At most ten branches, then "and N more", unless all.
func TestListBranchesIsCut(t *testing.T) {
	var b []string
	for i := 0; i < 317; i++ {
		b = append(b, fmt.Sprintf("feature/%d", i))
	}
	got := listBranches(b, false)
	if strings.Count(got, "feature/") != 10 || !strings.HasSuffix(got, "and 307 more") {
		t.Errorf("cut list: %q", got)
	}
	if strings.Count(listBranches(b, true), "feature/") != 317 {
		t.Error("--all does not list them all")
	}
}
