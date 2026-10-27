# Copy this file to your tap repository (github.com/lcajigasm/homebrew-omega,
# path Formula/omega.rb) and update url/sha256 on every release:
#   make formula VERSION=0.1.0
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  url "https://github.com/lcajigasm/omega/archive/refs/tags/v0.4.0.tar.gz"
  sha256 "5e47b433903d771e8ac3bdb6f37a6d1ae1d39b02602afa14426319f804906f66"
  license "MIT"
  head "https://github.com/lcajigasm/omega.git", branch: "main"

  depends_on "go" => :build
  depends_on :macos

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=#{version}"), "./cmd/omega"
    generate_completions_from_executable(bin/"omega", "completion", shell_parameter_format: :cobra)
  end

  def caveats
    <<~EOS
      Configure a provider and API key with:
        omega setup
      topgrade and Mole are optional backends:
        brew install topgrade mole
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/omega --version")
    assert_match "brew", shell_output("#{bin}/omega steps")
  end
end
