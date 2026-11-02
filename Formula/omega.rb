# Installed from the release archives. The release workflow rewrites version,
# urls and sha256 when a tag is published. The Homebrew taps refresh their own
# copy of this file; this repository does not push to them. verify-taps checks
# that both copies match the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.12.0"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.12.0/omega_0.12.0_darwin_arm64.tar.gz"
      sha256 "2e3a586f637d472c3eaefd3af9fbb1799f38ae9d2ab41573f50bf37c2afdf12d"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.12.0/omega_0.12.0_darwin_amd64.tar.gz"
      sha256 "1fbfedd7bad49dacb99b53cf4b6e4caef6f8cf61cd87a00b7f9c991d603c0145"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.12.0/omega_0.12.0_linux_arm64.tar.gz"
      sha256 "e4b3d78c08834bd4c5393e6a4dc336ad40dcdd62f65120ac8a8ea9fb1feb2432"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.12.0/omega_0.12.0_linux_amd64.tar.gz"
      sha256 "d76a1f9bfcb31783e4c3b24984f9e96460ffb381a4345f22ada275bf79fc32ce"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.12.0_windows_amd64.zip sha256 9845c9f150c1000755aeedfd624948dae0dc67a29bae6648f57a35299a173d69
  # omega_0.12.0_windows_arm64.zip sha256 b2f30129013df6c8d096e7a63d29a3fe595a1dc742ef1ab2c68a5ce756f54e08

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
