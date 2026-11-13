# Template, not an install path. brew install uses lcajigasm/homebrew-tap,
# which writes its own formula from each release. The release workflow
# rewrites version, urls and sha256 here when a version is published, and
# verify-taps checks the tap copy against the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.25.0"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.25.0/omega_0.25.0_darwin_arm64.tar.gz"
      sha256 "267631e4c8eb1281fff5af3329cc2cf5773b87a3cfb91867430f0a765883e3b7"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.25.0/omega_0.25.0_darwin_amd64.tar.gz"
      sha256 "d1baa4f429b5f6988d0151afadecdb69d76d6bee1e4598ec86bd8be04e0df43e"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.25.0/omega_0.25.0_linux_arm64.tar.gz"
      sha256 "8f5d6d7db88a15d56b9daefc16c75556ddd60644753c63eefbd7e8a3a4e9afca"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.25.0/omega_0.25.0_linux_amd64.tar.gz"
      sha256 "f85a3790b22019ac9daf282610e77bb26f6fe37992baa4d08c4db4c41d378e0a"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.25.0_windows_amd64.zip sha256 d89633a986c6b40516167aa8ccbcaad8edf848a2ddd0e7c4d44ea87bd4fde216
  # omega_0.25.0_windows_arm64.zip sha256 d5e1c04c5b99ca0b055c198347c35588398926c842f9c95f6531045f93ab2b74

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
