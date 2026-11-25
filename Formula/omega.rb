# Template, not an install path. brew install uses lcajigasm/homebrew-tap,
# which writes its own formula from each release. The release workflow
# rewrites version, urls and sha256 here when a version is published, and
# verify-taps checks the tap copy against the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.33.0"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.33.0/omega_0.33.0_darwin_arm64.tar.gz"
      sha256 "d5eeae0cc8bec354780566c0f270b94ffc66b686b9172632c5a24d600f79d5c2"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.33.0/omega_0.33.0_darwin_amd64.tar.gz"
      sha256 "700de0ecfd100256377ae617bdf3ce1b9331d991f4a641573638a7e894ea2ca4"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.33.0/omega_0.33.0_linux_arm64.tar.gz"
      sha256 "375205536fecd066482304aa7baec9989504a33a475f9b6cb3995df222fb77ad"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.33.0/omega_0.33.0_linux_amd64.tar.gz"
      sha256 "5085cb53174db7327138d0e5dc920748e7ad1a7c0ccf08ab5f49524fad1f19ad"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.33.0_windows_amd64.zip sha256 dba4af767ebb5f48987e5880cbab404beb212b139a30095518ae946cc89d7b4e
  # omega_0.33.0_windows_arm64.zip sha256 9abc9ef3bda48020d42daaf90558140ff32796c34afa1cc17411aad3e49d78ae

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
