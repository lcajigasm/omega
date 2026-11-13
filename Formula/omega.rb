# Template, not an install path. brew install uses lcajigasm/homebrew-tap,
# which writes its own formula from each release. The release workflow
# rewrites version, urls and sha256 here when a version is published, and
# verify-taps checks the tap copy against the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.26.0"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.26.0/omega_0.26.0_darwin_arm64.tar.gz"
      sha256 "cff8773fccc9aebf699c2b372dedc5fc04840e6beff563bfd8075c599c67cf53"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.26.0/omega_0.26.0_darwin_amd64.tar.gz"
      sha256 "0c9772e0274fcb3d0944324c29878b068cf09ee2996af43b2fef07891112f740"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.26.0/omega_0.26.0_linux_arm64.tar.gz"
      sha256 "2dfde83d87abd6209530d536bb0e6fa3b0a8917199eb1417ed7a204a87552ba8"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.26.0/omega_0.26.0_linux_amd64.tar.gz"
      sha256 "d7072744f0ea2ab8fe35e85d3b685d9e8a64a575f9df8ca0896b4e224b5eb3c4"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.26.0_windows_amd64.zip sha256 7b1866da996f5093c212532ab536c8f9d0886cb72e0b204017782e19dae17009
  # omega_0.26.0_windows_arm64.zip sha256 c1c8b9c3ad2f8f22c58da3f70eb24fd8f4fce59baec8e788d0e0cd9fd474bb12

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
