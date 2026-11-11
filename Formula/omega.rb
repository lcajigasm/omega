# Template, not an install path. brew install uses lcajigasm/homebrew-tap,
# which writes its own formula from each release. The release workflow
# rewrites version, urls and sha256 here when a version is published, and
# verify-taps checks the tap copy against the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.19.0"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.19.0/omega_0.19.0_darwin_arm64.tar.gz"
      sha256 "c47d7eddaa1da1739473730791d0dbee29ff25810845a4d103498328f098693a"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.19.0/omega_0.19.0_darwin_amd64.tar.gz"
      sha256 "0f0c38c5251175a4292415f10784cfcc65c454b14f317bb70e3619f4d0cabb7b"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.19.0/omega_0.19.0_linux_arm64.tar.gz"
      sha256 "75bf7b42b368acac10188f9694e06f9f1000369b613a032748b8934113487636"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.19.0/omega_0.19.0_linux_amd64.tar.gz"
      sha256 "a02a6a0000cc318b8d9cf8e525661cb50d6b066a6258bcbea86b450453afac58"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.19.0_windows_amd64.zip sha256 ba8953c12b57fd25817d9eba0402273b36b1710ed384357bff4012b72e070362
  # omega_0.19.0_windows_arm64.zip sha256 904c54de4767c0aaf76ea09f4a574df439042d12aea01e2b1cb6b1232d752589

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
