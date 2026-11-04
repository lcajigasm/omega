# Installed from the release archives. The release workflow rewrites version,
# urls and sha256 when a tag is published. The Homebrew taps refresh their own
# copy of this file; this repository does not push to them. verify-taps checks
# that both copies match the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.13.0"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.13.0/omega_0.13.0_darwin_arm64.tar.gz"
      sha256 "732cf4caabecd68adf0c0d6a1186be2ec648a53880eb6afa09f326b062931693"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.13.0/omega_0.13.0_darwin_amd64.tar.gz"
      sha256 "20faad80ac62cd3ca61e4d8aec8387079bbcb96b9a0f66900b5eb9d6b08eadee"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.13.0/omega_0.13.0_linux_arm64.tar.gz"
      sha256 "f6d8c26ecadff497c3cd1739e8516a30077dbdae0f70a11f67d0eeef7e146488"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.13.0/omega_0.13.0_linux_amd64.tar.gz"
      sha256 "849782ea95ac5c874d4d7997c06ae78f894b7738ac4bedb546b6834b24bc910b"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.13.0_windows_amd64.zip sha256 17750a1b4cf906e11695e54570975b5adc00b3d2747cb5a633b6cfed0bde7181
  # omega_0.13.0_windows_arm64.zip sha256 0d6bd20952d0870549ce53a02330af7b4026c1234bf56c4829473adcf096ed0d

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
