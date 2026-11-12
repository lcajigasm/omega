# Template, not an install path. brew install uses lcajigasm/homebrew-tap,
# which writes its own formula from each release. The release workflow
# rewrites version, urls and sha256 here when a version is published, and
# verify-taps checks the tap copy against the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.23.0"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.23.0/omega_0.23.0_darwin_arm64.tar.gz"
      sha256 "6366a41fa183c1ed4804936f17594d1c01636b81d0d636d1fab2bc918bfbef7d"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.23.0/omega_0.23.0_darwin_amd64.tar.gz"
      sha256 "5630bab996271c14390a47371b37dbb44d70381fd07db8702a20b078b99d3285"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.23.0/omega_0.23.0_linux_arm64.tar.gz"
      sha256 "376671ea415d1b4a9595fdc6bc0288e8f9ff72649d52d0fc1a52ef1b959c142d"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.23.0/omega_0.23.0_linux_amd64.tar.gz"
      sha256 "9752040650d988c28fc4432fcfc30b2c6d11cda81d555bc146f4225bff97cd5e"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.23.0_windows_amd64.zip sha256 f67360f45b882813d73e5aaac6072864c93d8c9a202394b4907b8acdd8964cb0
  # omega_0.23.0_windows_arm64.zip sha256 08bebe96e9204b3eda00ff4b5295a9ef78f22262962bf9b7b816a4fd73a90d3d

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
