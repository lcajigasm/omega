# Template, not an install path. brew install uses lcajigasm/homebrew-tap,
# which writes its own formula from each release. The release workflow
# rewrites version, urls and sha256 here when a version is published, and
# verify-taps checks the tap copy against the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.28.2"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.28.2/omega_0.28.2_darwin_arm64.tar.gz"
      sha256 "5dea012b0e4eae3c3367de1ac8454c1f971165a75cebebf4ff2063f4bbef0ab1"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.28.2/omega_0.28.2_darwin_amd64.tar.gz"
      sha256 "2ca1afe1067a84b350b9fa4ce2d7b17b2870182ce8675b451de52f0b414803a9"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.28.2/omega_0.28.2_linux_arm64.tar.gz"
      sha256 "945836986ebdcfd21ee19238a3a96a6be78bbe37ed5e47b64afb857f9fe76a7f"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.28.2/omega_0.28.2_linux_amd64.tar.gz"
      sha256 "35dd5f6f283340e9377d2283a83ae624d1acbf4d7c5d1aa07d54cc1a34efe4fd"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.28.2_windows_amd64.zip sha256 be851418291f4a3b9d531ba2c7f0722736b59963b419526f048d61f0ea8f8bc0
  # omega_0.28.2_windows_arm64.zip sha256 23f38247a81a7d54892ffd6fe04945a37d0b2eb4958ee11a4798938db226b059

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
