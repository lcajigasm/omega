#!/usr/bin/env bash
# Build Omega.app as a universal binary and wrap it in a disk image.
#
#   scripts/package_macos.sh VERSION [OUTDIR]     # OUTDIR defaults to dist
#
# The release workflow runs this on a macOS runner; `make gui-dmg` runs it
# here. The app is ad-hoc signed: enough for Apple Silicon to start it, not
# enough for Gatekeeper, which needs a Developer ID and notarization.
set -euo pipefail

ver="${1:?usage: scripts/package_macos.sh VERSION [OUTDIR]}"
ver="${ver#v}"
out="${2:-dist}"
cd "$(dirname "$0")/.."

export CGO_ENABLED=1 MACOSX_DEPLOYMENT_TARGET=13.0
export CGO_CFLAGS=-mmacosx-version-min=13.0 CGO_LDFLAGS=-mmacosx-version-min=13.0

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

for arch in arm64 amd64; do
  GOOS=darwin GOARCH="$arch" go build -trimpath -tags gui,production \
    -ldflags "-s -w -X main.version=v${ver}" -o "$work/omega-gui-$arch" ./cmd/omega-gui
done

app="$work/stage/Omega.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
lipo -create -output "$app/Contents/MacOS/omega-gui" "$work/omega-gui-arm64" "$work/omega-gui-amd64"
sed "s/@VERSION@/${ver}/g" cmd/omega-gui/Info.plist > "$app/Contents/Info.plist"
cp cmd/omega-gui/omega.icns "$app/Contents/Resources/omega.icns"
codesign --force --sign - "$app"

ln -s /Applications "$work/stage/Applications"
mkdir -p "$out"
hdiutil create -quiet -volname omega -srcfolder "$work/stage" -ov -format UDZO "$out/omega-macos-universal.dmg"
echo "$out/omega-macos-universal.dmg"
