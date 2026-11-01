# Installed from the release archives. The release workflow rewrites version,
# urls and sha256 when a tag is published. The Homebrew taps refresh their own
# copy of this file; this repository does not push to them. verify-taps checks
# that both copies match the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.11.0"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.11.0/omega_0.11.0_darwin_arm64.tar.gz"
      sha256 "fc279464337732d3bc1d4eee05b761696ac410900dba21d47ef608f46a90b2c8"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.11.0/omega_0.11.0_darwin_amd64.tar.gz"
      sha256 "4ed103ee4b9277b79705722df2ca8ae03481c85ddedfe2c1db759a2fc5e61a68"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.11.0/omega_0.11.0_linux_arm64.tar.gz"
      sha256 "9165b46b3702f9f3332e77f5ff0d11b36b5955e80367599b6c11c0f8da495f23"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.11.0/omega_0.11.0_linux_amd64.tar.gz"
      sha256 "bf206a98d8d635cfc623f052eb353b9a440b18d6357159056381e51bf6adeb6e"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.11.0_windows_amd64.zip sha256 2641e0df0a8b8486687643e2f3aaa68f3a93a5ab6e231d0c0339c3add6713424
  # omega_0.11.0_windows_arm64.zip sha256 866bb50666f0b11cd76b46d5eb50335f62e0b2ed526ed9b37d4e07d2dd579ab8

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
