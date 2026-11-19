# Template, not an install path. brew install uses lcajigasm/homebrew-tap,
# which writes its own formula from each release. The release workflow
# rewrites version, urls and sha256 here when a version is published, and
# verify-taps checks the tap copy against the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.30.0"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.30.0/omega_0.30.0_darwin_arm64.tar.gz"
      sha256 "d4e4268328f521a39be69a3362de509c09dd8e9d72521f0946d12d254203029e"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.30.0/omega_0.30.0_darwin_amd64.tar.gz"
      sha256 "d7a9de1dd60c7f8e90256b9518f67bc168f051a5e1b133f42a35b99681dcff94"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.30.0/omega_0.30.0_linux_arm64.tar.gz"
      sha256 "d0925aa89c8705f2e55852069b88f9d084264a4ba7a342835c34480481ce43bd"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.30.0/omega_0.30.0_linux_amd64.tar.gz"
      sha256 "dfd4e0648262864ed697469317758a23c4a0b141e2a7d4da7353163f8b202086"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.30.0_windows_amd64.zip sha256 15d9cd90dab606d2c90eb2b921d7a23664d84ad228e5f4511e447a87ebde24b2
  # omega_0.30.0_windows_arm64.zip sha256 6854922febf382a246a8335e954ab8dc31849dbdbd9519952d3d68f08049c82d

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
