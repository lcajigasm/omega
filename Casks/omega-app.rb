# The desktop app, for macOS. brew install uses lcajigasm/homebrew-tap: the
# release workflow rewrites version and sha256 here when a tag is published
# and copies this file to that tap in the same pull request as the formula.
cask "omega-app" do
  version "0.17.0"
  sha256 "bcc4abbb61f0dece911d634bcb367012aaa06ca4cb35bc30c3994780a99a3eef"

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

  depends_on macos: ">= :ventura"

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
