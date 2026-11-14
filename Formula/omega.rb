# Template, not an install path. brew install uses lcajigasm/homebrew-tap,
# which writes its own formula from each release. The release workflow
# rewrites version, urls and sha256 here when a version is published, and
# verify-taps checks the tap copy against the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.27.0"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.27.0/omega_0.27.0_darwin_arm64.tar.gz"
      sha256 "21833819b2ecf8a6450fd325f5808119c21862674b149bcd0aed3f22cc86b441"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.27.0/omega_0.27.0_darwin_amd64.tar.gz"
      sha256 "b6e5f0bc079b23f57afca5ca1bdbb699e36b0ce8edb8e6d5f9d8e52c248bb743"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.27.0/omega_0.27.0_linux_arm64.tar.gz"
      sha256 "c0011850183bbb9d10f93b4574a002b55ad356defcb66f972227109dc85c67be"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.27.0/omega_0.27.0_linux_amd64.tar.gz"
      sha256 "cf98d29c0faa434e3099f53a3d1f8633eee482903773e9cbddef267c4e7702ef"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.27.0_windows_amd64.zip sha256 b96dbe1255c6c4ecd051f8c3306d8e0c815cffcfb32fbc5f7bed40060c4bcf3b
  # omega_0.27.0_windows_arm64.zip sha256 a39712d1588b65760fdc57de7654ac9955e0ad1b860bf1af5841e6864a3fb2d7

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
