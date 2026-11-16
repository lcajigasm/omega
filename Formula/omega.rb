# Template, not an install path. brew install uses lcajigasm/homebrew-tap,
# which writes its own formula from each release. The release workflow
# rewrites version, urls and sha256 here when a version is published, and
# verify-taps checks the tap copy against the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.28.1"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.28.1/omega_0.28.1_darwin_arm64.tar.gz"
      sha256 "7aaa3d4a5b75023e0ffdd0ff5ce8fc8450577190a900cdccc480751250693bd7"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.28.1/omega_0.28.1_darwin_amd64.tar.gz"
      sha256 "66ba871b79e5c090f791ad68aea97b3086abbccde776f024e66581196376dab8"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.28.1/omega_0.28.1_linux_arm64.tar.gz"
      sha256 "5ccdf3b5f082503caf8fd7c646564af213e61098ff09899df4f8607e74b6e470"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.28.1/omega_0.28.1_linux_amd64.tar.gz"
      sha256 "04e54fbb3096aebb3bef9e204ed2ed6f57c838dbecf8daf7155e29967145ca15"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.28.1_windows_amd64.zip sha256 72224be1e60e85b68d13076af88f7d9e99b1f7382c74a8edf0527e1ea7997122
  # omega_0.28.1_windows_arm64.zip sha256 6af17577150418e2c282fd985345af4a5facdd685461b4c42c74678a77899320

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
