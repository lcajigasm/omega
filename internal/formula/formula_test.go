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

func TestPatchTapFormula(t *testing.T) {
	const src = `class Omega < Formula
  desc "AI-driven maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.15.0/omega_0.15.0_darwin_arm64.tar.gz"
      sha256 "7145688db5944e3f3b7000856842a9c719c1b3889af85db822fc3274fde9a54e"
    end

    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.15.0/omega_0.15.0_darwin_amd64.tar.gz"
      sha256 "c94a68037fd0d30b3365cd571ec0cf5f97830f6f38c4039e3842522370b447c7"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.15.0/omega_0.15.0_linux_arm64.tar.gz"
      sha256 "522a64d09a4ab16da34777aa36c2cc876b019e0b71750b3229304d9141d70883"
    end

    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.15.0/omega_0.15.0_linux_amd64.tar.gz"
      sha256 "d7dc6e0ff2a6dc43e0b23e4044b7c200d1fd6d6b58ef46b7edaa7fe5a12a113e"
    end
  end

  def install
    bin.install "omega"
  end
end
`
	sums := map[string]string{}
	for i, name := range HomebrewArchives("9.9.9") {
		sums[name] = strings.Repeat(string(rune('a'+i)), 64)
	}
	out, err := Patch(src, "9.9.9", sums)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "0.15.0") || strings.Contains(out, `version "`) {
		t.Fatalf("tap formula still has the old version or gained a version line:\n%s", out)
	}
	if !strings.Contains(out, "AI-driven maintenance") || !strings.Contains(out, `bin.install "omega"`) {
		t.Fatal("patch rewrote more than the version and the checksums")
	}
	for _, name := range HomebrewArchives("9.9.9") {
		if !strings.Contains(out, "v9.9.9/"+name) || !strings.Contains(out, sums[name]) {
			t.Fatalf("formula is missing %s", name)
		}
	}
	if _, err := Patch(src, "9.9.9", map[string]string{}); err == nil {
		t.Fatal("missing checksums were accepted")
	}
	full := map[string]string{}
	for _, name := range Archives("9.9.9") {
		full[name] = strings.Repeat("ab", 32)
	}
	if _, err := Update(src, "9.9.9", full); err == nil {
		t.Fatal("tap formula was accepted as the template")
	}
}

func TestPatchLeavesWindowsAlone(t *testing.T) {
	const windowsURL = `url "https://github.com/lcajigasm/omega/releases/download/v0.15.0/omega_0.15.0_windows_amd64.zip"`
	const windowsSHA = `sha256 "1111111111111111111111111111111111111111111111111111111111111111"`
	const windowsComment = `# omega_0.15.0_windows_arm64.zip sha256 2222222222222222222222222222222222222222222222222222222222222222`
	src := `class Omega < Formula
  desc "AI-driven maintenance"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.15.0/omega_0.15.0_darwin_arm64.tar.gz"
      sha256 "7145688db5944e3f3b7000856842a9c719c1b3889af85db822fc3274fde9a54e"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.15.0/omega_0.15.0_darwin_amd64.tar.gz"
      sha256 "c94a68037fd0d30b3365cd571ec0cf5f97830f6f38c4039e3842522370b447c7"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.15.0/omega_0.15.0_linux_arm64.tar.gz"
      sha256 "522a64d09a4ab16da34777aa36c2cc876b019e0b71750b3229304d9141d70883"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.15.0/omega_0.15.0_linux_amd64.tar.gz"
      sha256 "d7dc6e0ff2a6dc43e0b23e4044b7c200d1fd6d6b58ef46b7edaa7fe5a12a113e"
    end
  end

  ` + windowsURL + `
  ` + windowsSHA + `
  ` + windowsComment + `
end
`
	sums := map[string]string{}
	for i, name := range Archives("9.9.9") {
		sums[name] = strings.Repeat(string(rune('a'+i)), 64)
	}
	out, err := Patch(src, "9.9.9", sums)
	if err != nil {
		t.Fatal(err)
	}
	for _, kept := range []string{windowsURL, windowsSHA, windowsComment} {
		if !strings.Contains(out, kept) {
			t.Fatalf("tap patch rewrote a windows archive:\n%s", out)
		}
	}
	if strings.Contains(out, "omega_9.9.9_windows") {
		t.Fatalf("tap patch rewrote a windows archive:\n%s", out)
	}
}
