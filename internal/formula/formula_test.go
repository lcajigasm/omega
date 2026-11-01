package formula

import (
	"os"
	"strings"
	"testing"
)

func TestParseSums(t *testing.T) {
	sums, err := ParseSums("\n# comment\n" + strings.Repeat("ab", 32) + "  omega_1.2.3_darwin_arm64.tar.gz\n")
	if err != nil {
		t.Fatal(err)
	}
	if sums["omega_1.2.3_darwin_arm64.tar.gz"] != strings.Repeat("ab", 32) {
		t.Fatalf("sums = %v", sums)
	}
	if _, err := ParseSums("nope\n"); err == nil {
		t.Fatal("bad line was accepted")
	}
}

func TestUpdateRepoFormula(t *testing.T) {
	src, err := os.ReadFile("../../Formula/omega.rb")
	if err != nil {
		t.Fatal(err)
	}
	sums := map[string]string{}
	for _, name := range Archives("9.9.9") {
		sums[name] = strings.Repeat("cd", 32)
	}
	// Distinct hashes so a swapped platform fails the test.
	for i, name := range Archives("9.9.9") {
		sums[name] = strings.Repeat(string(rune('a'+i)), 64)
	}
	out, err := Update(string(src), "9.9.9", sums)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "0.9.0") {
		t.Fatal("old version still in the formula")
	}
	if !strings.Contains(out, `version "9.9.9"`) {
		t.Fatal("version was not rewritten")
	}
	for name, sum := range sums {
		if !strings.Contains(out, name) || !strings.Contains(out, sum) {
			t.Fatalf("formula is missing %s %s", name, sum)
		}
	}
	if _, err := Update(string(src), "9.9.9", map[string]string{}); err == nil {
		t.Fatal("missing checksums were accepted")
	}
	if _, err := Update(string(src), "v9.9.9", sums); err == nil {
		t.Fatal("v prefix was accepted")
	}
}
