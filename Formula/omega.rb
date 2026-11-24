# Template, not an install path. brew install uses lcajigasm/homebrew-tap,
# which writes its own formula from each release. The release workflow
# rewrites version, urls and sha256 here when a version is published, and
# verify-taps checks the tap copy against the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.32.1"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.32.1/omega_0.32.1_darwin_arm64.tar.gz"
      sha256 "cfbb877b11d031be212a9836dcac30f8ff1d7fcf8c84007d21c43af0aeb26123"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.32.1/omega_0.32.1_darwin_amd64.tar.gz"
      sha256 "1e7c3a51ae73a15899715ee98f6e74a262b10eb5e8bff1b22880c06fcda82483"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.32.1/omega_0.32.1_linux_arm64.tar.gz"
      sha256 "c4ad0feb14649986394bcaf34acfa38088fc3fd37ac89df9b56429678a36ae7b"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.32.1/omega_0.32.1_linux_amd64.tar.gz"
      sha256 "763acb80e8b2ff0a2901b9978b80be4734f9934aadc23f312cda68736039b22f"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.32.1_windows_amd64.zip sha256 4914a5eba0b79ce73795d551bcc504de83b951c95418ce55bf311b46bc7cb38a
  # omega_0.32.1_windows_arm64.zip sha256 febbbddc80dca77aac8661c709bafb86be7745d47628bf4886b3ee7aa4fba1af

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
