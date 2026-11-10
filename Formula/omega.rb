# Template, not an install path. brew install uses lcajigasm/homebrew-tap.
# The release workflow rewrites version, urls and sha256 here when a tag is
# published, and opens a pull request on that tap with the same version and
# the four Homebrew sha256. verify-taps checks the tap copy against the tag.
class Omega < Formula
  desc "AI-driven macOS maintenance: updates like topgrade, cleans like Mole"
  homepage "https://github.com/lcajigasm/omega"
  license "MIT"
  version "0.16.0"

  on_macos do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.16.0/omega_0.16.0_darwin_arm64.tar.gz"
      sha256 "da1f5fb4bedff5a50b996f3c7681a02981aa4dbb041027fd204f748925355751"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.16.0/omega_0.16.0_darwin_amd64.tar.gz"
      sha256 "005e348ca11139313ac4769000e647031b3495554df4f3d0cc6f6039ea4ddaed"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lcajigasm/omega/releases/download/v0.16.0/omega_0.16.0_linux_arm64.tar.gz"
      sha256 "f64cdde9fd1b25cdb89584b3e577e2fbb71063b801d8fc18fd10f8030993ea34"
    end
    on_intel do
      url "https://github.com/lcajigasm/omega/releases/download/v0.16.0/omega_0.16.0_linux_amd64.tar.gz"
      sha256 "ed5d270833b81d6ab6e20dba392f3b63c047c06503ca0cab55ff995ed1326db1"
    end
  end

  # Windows archives are not installed by Homebrew. The release workflow
  # rewrites these checksums together with the urls above.
  # omega_0.16.0_windows_amd64.zip sha256 4c385141226213d660ffcd287761f202cae90d15d82179097b1f5df1d09d9739
  # omega_0.16.0_windows_arm64.zip sha256 c4b28f5a652d75443c4e9daea839d7b553ea764504b3001ebe882a840f78c136

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
