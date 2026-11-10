# Template, not an install path. brew install uses lcajigasm/homebrew-tap.
# The release workflow rewrites version, urls and sha256 here when a tag is
# published, and opens a pull request on that tap with the same version and
# the four Homebrew sha256. verify-taps checks the tap copy against the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.15.0"

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

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.15.0_windows_amd64.zip sha256 c3798c1a1fbd661e6d5afdef41c219f8bd255f9b46dd624db7c93b2561438eac
  # omega_0.15.0_windows_arm64.zip sha256 8551ba7556c0c4f924a53297f98370a194733164814952ad96395572312580d4

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
