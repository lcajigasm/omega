# The desktop app, for macOS. brew install uses lcajigasm/homebrew-tap, which
# writes its own copy from each release. The release workflow records the
# version and the sha256 here too, so this file shows what was published.
cask "omega-app" do
  version "0.24.0"
  sha256 "bb4ff5d8165f7abc4b433ec2a15b363004b00b772364948db782a34576f5c4bf"

  url "https://github.com/lcajigasm/omega/releases/download/v#{version}/omega-macos-universal.dmg"
  name "omega"
  desc "AI-driven maintenance: updates like topgrade, cleans like Mole, in a window"
  homepage "https://github.com/lcajigasm/omega"

  livecheck do
    url :url
    strategy :github_releases do |json, regex|
      json.map do |release|
        next if release["draft"] || release["prerelease"]

        assets = release.fetch("assets", []).map { |asset| asset["name"] }
        next unless assets.include?("omega-macos-universal.dmg")

        match = release["tag_name"]&.match(regex)
        next unless match

        match[1]
      end
    end
  end

  depends_on macos: :ventura

  app "Omega.app"

  # No zap: ~/.config/omega and ~/.local/state/omega belong to the omega
  # command too (brew install lcajigasm/tap/omega), which shares them.

  caveats <<~EOS
    Omega.app is ad-hoc signed and is not notarized, so Gatekeeper
    quarantines it. Clear the quarantine attribute after install:

      xattr -dr com.apple.quarantine /Applications/Omega.app

    The app and the omega command share their configuration and API keys.
    The command is a separate formula:

      brew install lcajigasm/tap/omega
  EOS
end
