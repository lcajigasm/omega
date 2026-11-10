# Template, not an install path. brew install uses lcajigasm/homebrew-tap.
# The release workflow rewrites version, urls and sha256 here when a tag is
# published, and opens a pull request on that tap with the same version and
# the four Homebrew sha256. verify-taps checks the tap copy against the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.17.0"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.17.0/omega_0.17.0_darwin_arm64.tar.gz"
      sha256 "2286af2c5278057831f1db58eb89b2facf1d96e4c0f1ec21bec550d803f7887f"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.17.0/omega_0.17.0_darwin_amd64.tar.gz"
      sha256 "ec361b215b2082b2691b61aa75bb94197420376df1cf98b76c2d49e3ba7d290a"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.17.0/omega_0.17.0_linux_arm64.tar.gz"
      sha256 "f7dba10643177b11fd53fdd560f80843db4cf731066708ef37c163523e0c94c7"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.17.0/omega_0.17.0_linux_amd64.tar.gz"
      sha256 "93ec7e33d6a9ff8cdc83d59b129a3b2beec20950c0d68395f752a809ff0f7975"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.17.0_windows_amd64.zip sha256 c082fd83fc029e6579655db2b4ff0b652d5216bfbef2de08583a26d14caa625b
  # omega_0.17.0_windows_arm64.zip sha256 59670af3a03897b113c8e05c4f318f0e94ed0996ba0b7357afabedcf4ae08ffc

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
