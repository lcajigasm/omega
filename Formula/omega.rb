# Installed from the release archives. The release workflow rewrites version,
# urls and sha256 when a tag is published. The Homebrew taps refresh their own
# copy of this file; this repository does not push to them. verify-taps checks
# that both copies match the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.14.0"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.14.0/omega_0.14.0_darwin_arm64.tar.gz"
      sha256 "c550f02bcdbe827f01bd01dc7aa0f3938a20b40456b518f99f7e65c62d9d2e06"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.14.0/omega_0.14.0_darwin_amd64.tar.gz"
      sha256 "15b151d0551361912b175a1a2f9f43bf6bd12622d1ba907d3102b9f7a8ef3c23"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.14.0/omega_0.14.0_linux_arm64.tar.gz"
      sha256 "9f174df3f52061a129bab6d7478bb94009c316da5043f36979442793bb1f42e9"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.14.0/omega_0.14.0_linux_amd64.tar.gz"
      sha256 "c36608b726fbfb5c2ac969a297232c71e74dd984854f9208789cf7d1c0840556"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.14.0_windows_amd64.zip sha256 817a76199340cf3c077b2ead982a4971cbee975b303c49c126806c798bddaadb
  # omega_0.14.0_windows_arm64.zip sha256 c0a69b3662565df73aeae640a3aa79b6e0feb8600f200adc28e1b014efcf9fd2

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
