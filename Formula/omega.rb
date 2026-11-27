# Template, not an install path. brew install uses lcajigasm/homebrew-tap,
# which writes its own formula from each release. The release workflow
# rewrites version, urls and sha256 here when a version is published, and
# verify-taps checks the tap copy against the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.37.0"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.37.0/omega_0.37.0_darwin_arm64.tar.gz"
      sha256 "d1df3af4e9733fd58ffa36fe59b73b68ebcb7045d49fa0c8b1309f9da175a89d"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.37.0/omega_0.37.0_darwin_amd64.tar.gz"
      sha256 "0ca8eddb697feaa4e2371b62d427d8c05396a3a873d537cb8e3ce01a7dca3b22"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.37.0/omega_0.37.0_linux_arm64.tar.gz"
      sha256 "b945d4eab2563086166764a1ceac0837e87c3b7c3727bb41779b6bd4ac61b179"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.37.0/omega_0.37.0_linux_amd64.tar.gz"
      sha256 "210b7c1f41b605f9cdac8ab9440419b73eddc8dcb3c6ef71c39c6241b6c9cd98"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.37.0_windows_amd64.zip sha256 c6045e70d9b2229bf34c798b1b11297893f905b65d5a5d24a394dcc6a4d59531
  # omega_0.37.0_windows_arm64.zip sha256 cb94926c2bcfc3a5bec599df3ffc2d09aa2a4e084b00d217a863f5101bfbc866

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
