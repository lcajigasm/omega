# Template, not an install path. brew install uses lcajigasm/homebrew-tap,
# which writes its own formula from each release. The release workflow
# rewrites version, urls and sha256 here when a version is published, and
# verify-taps checks the tap copy against the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.22.1"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.22.1/omega_0.22.1_darwin_arm64.tar.gz"
      sha256 "508506fd13e2bce9d9a081945d94a6bce30eec8fc283b81e2c12a687b2605333"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.22.1/omega_0.22.1_darwin_amd64.tar.gz"
      sha256 "289cb98f0cd9dea7ca7282698fd9ec9bd4014edabe4ef4d0f15faaef7d50c4c3"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.22.1/omega_0.22.1_linux_arm64.tar.gz"
      sha256 "7fb79cda1cfb26f532de5f338a5f4e6c5bffb4f6f4c0870209d84b303ac56d77"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.22.1/omega_0.22.1_linux_amd64.tar.gz"
      sha256 "a36dbba9008732a8be84a9b73915dd764cbc3708751cddfd2c094a54d9b9b264"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.22.1_windows_amd64.zip sha256 affb428f96f02d99b4239d9d41c90179381164ca2eeb0b4d8dc1b4306e16d09b
  # omega_0.22.1_windows_arm64.zip sha256 f9802f6647917b22fb9883ec62fa7ae7c916a1c8700ee5f902fe8e8130462871

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
