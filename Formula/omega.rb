# Template, not an install path. brew install uses lcajigasm/homebrew-tap.
# The release workflow rewrites version, urls and sha256 here when a tag is
# published, and opens a pull request on that tap with the same version and
# the four Homebrew sha256. verify-taps checks the tap copy against the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.18.0"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.18.0/omega_0.18.0_darwin_arm64.tar.gz"
      sha256 "583f4e0d142493b82472c6541593860c03adb57b368087b3092bdba30522cc32"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.18.0/omega_0.18.0_darwin_amd64.tar.gz"
      sha256 "1526ade4a9624a1552a12c6e0847702a15fce7787e546425313c2d44dff81894"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.18.0/omega_0.18.0_linux_arm64.tar.gz"
      sha256 "de0ca9d0083260de228db7edc6ac9a8fab5c7e962f2fd5cc6d0953bc95dbcfbd"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.18.0/omega_0.18.0_linux_amd64.tar.gz"
      sha256 "e158c045b670ea3dec56d10ac88a07d6aed2e68be134e82f10d33226221376ac"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.18.0_windows_amd64.zip sha256 6d4dfebeb85735734e76bf59376cc6238223459c17e654dcafc6a49aa93ca449
  # omega_0.18.0_windows_arm64.zip sha256 6742fba33accfc9531a67afc12404fabd052dbbf4af0807226e06869edae0743

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
