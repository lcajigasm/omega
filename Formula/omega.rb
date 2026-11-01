# Installed from the release archives. The release workflow rewrites version,
# urls and sha256 when a tag is published. Copy this file to
# github.com/lcajigasm/homebrew-omega (Formula/omega.rb). That tap is a
# separate repository and is not updated from here.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.10.0"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.10.0/omega_0.10.0_darwin_arm64.tar.gz"
      sha256 "e07c80c5c6057be3c3acd18632239a0110ebf0d74bcb517ce92f1551ea6f48bf"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.10.0/omega_0.10.0_darwin_amd64.tar.gz"
      sha256 "927886263140a1d8f51746e7cfbcb2718a84899ac8deaf1da7f6a40e18be090b"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.10.0/omega_0.10.0_linux_arm64.tar.gz"
      sha256 "d1c4c131e44376dae208d60e36a7052d5cd020c404884077042cd25280c75ddf"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.10.0/omega_0.10.0_linux_amd64.tar.gz"
      sha256 "5ca846181a3d93da63b58754566ee9af4bf5066ca944b2bd848eaabb3b8c4d26"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.10.0_windows_amd64.zip sha256 728bd318862ae73c8b693d280c8364eb76190526d3e1af5e4cc1f7d58844e549
  # omega_0.10.0_windows_arm64.zip sha256 708c1d69d9e481d0be7ff4351754bdea22df136383e13241e6175c864e2e6496

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
