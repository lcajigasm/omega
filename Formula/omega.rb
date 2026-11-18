# Template, not an install path. brew install uses lcajigasm/homebrew-tap,
# which writes its own formula from each release. The release workflow
# rewrites version, urls and sha256 here when a version is published, and
# verify-taps checks the tap copy against the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.29.0"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.29.0/omega_0.29.0_darwin_arm64.tar.gz"
      sha256 "076e9d1b0fe57bee94fcb55a80e56ff2f2ea5d862b1288138da04f0add0b10c6"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.29.0/omega_0.29.0_darwin_amd64.tar.gz"
      sha256 "17c2b9331dc737f6db7910281ebf1983b770e9516006b2bc6beee444f29e1f7a"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.29.0/omega_0.29.0_linux_arm64.tar.gz"
      sha256 "2a4c3a5444d48ee59d451ff55a767a8a6366647bc3bdc43d4ff20acff526fb6e"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.29.0/omega_0.29.0_linux_amd64.tar.gz"
      sha256 "19b94f8b959311077ba5f28d1c5ec2578bf9d64a3af0cdcc229bb606a3683be7"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.29.0_windows_amd64.zip sha256 378d10807f5e805620891a23f73b1b11f6a32985ae21f949689e4928b62d1d5e
  # omega_0.29.0_windows_arm64.zip sha256 8ff288687315c656c4795f0d4943b896d7d4ba78b273afa10befbe48dfced35b

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
