# Template, not an install path. brew install uses lcajigasm/homebrew-tap,
# which writes its own formula from each release. The release workflow
# rewrites version, urls and sha256 here when a version is published, and
# verify-taps checks the tap copy against the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.35.0"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.35.0/omega_0.35.0_darwin_arm64.tar.gz"
      sha256 "ab28dc1ac803ec3d041bb31d1fa6a41978688e939c5b553f9364b1dde0c78285"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.35.0/omega_0.35.0_darwin_amd64.tar.gz"
      sha256 "04d04c16cc241538e10639e2a1ad510e905f85a3c98170ab22d946021d5ac5e1"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.35.0/omega_0.35.0_linux_arm64.tar.gz"
      sha256 "379da5ddccbd118010d971e6ec37d28e8928bf6eea622a44f6bbc92944a54f1e"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.35.0/omega_0.35.0_linux_amd64.tar.gz"
      sha256 "7327fa77f53681a7bc691a66084a2045779c73a11baf1b64597341953b2055b2"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.35.0_windows_amd64.zip sha256 5a7034de2a0950ecd1a3a44b0dbac5bf35d0b74b62607d039b3da7d979bf1d39
  # omega_0.35.0_windows_arm64.zip sha256 3f2c5ccca15c083db5e361e62687c8f95995b2f8be3e441deb9f44d3a7422342

  def install
    bin.install "omega"
    generate_completions_from_executable(bin/"omega", "completion", shell_parameter_format: :cobra)
  end

  def caveats
    <<~EOS
      Configure a provider and API key with:
        omega setup
      Maintenance steps target macOS. topgrade and Mole are optional backends:
        brew install topgrade mole
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/omega --version")
    assert_match "brew", shell_output("#{bin}/omega steps")
  end
end
