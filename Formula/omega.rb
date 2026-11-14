# Template, not an install path. brew install uses lcajigasm/homebrew-tap,
# which writes its own formula from each release. The release workflow
# rewrites version, urls and sha256 here when a version is published, and
# verify-taps checks the tap copy against the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.26.1"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.26.1/omega_0.26.1_darwin_arm64.tar.gz"
      sha256 "900577eaca1238388b503cf8e64f3d272aa19039852ba9da2a6e1c78ec793fa3"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.26.1/omega_0.26.1_darwin_amd64.tar.gz"
      sha256 "9b8161ad9acc833fa9fd826066b37198bc2eed5551b35c33932d109785379f3d"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.26.1/omega_0.26.1_linux_arm64.tar.gz"
      sha256 "78e36c1d999c61ba1e5c369c23f6b242935d02206c29dbeb05888e2d99979e1f"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.26.1/omega_0.26.1_linux_amd64.tar.gz"
      sha256 "5014d92798ea478dd8b445313e8487e7121981397ef10ffbf0e8cb2984a5c6e4"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.26.1_windows_amd64.zip sha256 a212abcd01ae2ca9d5ace8a2d189fd345b1f095821958e4389754902f3fc86d0
  # omega_0.26.1_windows_arm64.zip sha256 70d71ba0ce51010a3c2945d5d99712bfb4829ec41558846d5dc24eca19f7edd7

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
