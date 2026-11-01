# Installed from the release archives. The release workflow rewrites version,
# urls and sha256 when a tag is published. Copy this file to
# github.com/lcajigasm/homebrew-omega (Formula/omega.rb). That tap is a
# separate repository and is not updated from here.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.9.0"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.9.0/omega_0.9.0_darwin_arm64.tar.gz"
      sha256 "25793356b6a7d8b0a19f4d6c7ba9ff3f9e1c1825ce38f5f34a0f96d1291c6073"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.9.0/omega_0.9.0_darwin_amd64.tar.gz"
      sha256 "b5a9a3ce629459959e40af19fd449a0cde09caae5e84621b83aedbc311c15226"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.9.0/omega_0.9.0_linux_arm64.tar.gz"
      sha256 "c82186ce4a569adf2cc04e2dd60a4b8dc5e52f52b368e82d096921b992c2fb37"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.9.0/omega_0.9.0_linux_amd64.tar.gz"
      sha256 "ed5bce555c8da302026a59fdf06fc4d6914ae8c05edbe94755ffbc60749ffc3c"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.9.0_windows_amd64.zip sha256 b257cfe546c9fed494820be4c94ce8a722fd49e1c114dae756a8501b9f613d40
  # omega_0.9.0_windows_arm64.zip sha256 40c43ea808d2d2917f332310da78e585ec51c414044991ae3a3039d18d497fb5

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
