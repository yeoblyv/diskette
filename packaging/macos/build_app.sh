#!/bin/bash
# Assembles Diskette.app: compiles the real binary for the given GOARCH,
# wraps it with the Terminal-launcher script (see diskette-launcher),
# and drops in Info.plist/AppIcon.icns. Run from the repo root:
#   packaging/macos/build_app.sh [arm64|amd64] [version]
#
# version defaults to the nearest git tag (stripped of its leading "v"),
# so a release build run from a tagged commit needs no second argument.
# It is the single value substituted into both the binary (via -ldflags,
# see cmd/diskette/version.go) and Info.plist's __VERSION__ placeholder —
# neither is ever hand-edited to bump a version.
set -euo pipefail

ARCH="${1:-arm64}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
VERSION="${2:-$(git -C "$ROOT" describe --tags --always --dirty | sed 's/^v//')}"
OUT="$ROOT/dist/Diskette.app"

rm -rf "$OUT"
mkdir -p "$OUT/Contents/MacOS" "$OUT/Contents/Resources"

sed "s/__VERSION__/$VERSION/g" "$ROOT/packaging/macos/Info.plist" > "$OUT/Contents/Info.plist"
cp "$ROOT/packaging/icons/AppIcon.icns" "$OUT/Contents/Resources/AppIcon.icns"
cp "$ROOT/packaging/macos/diskette-launcher" "$OUT/Contents/MacOS/diskette-launcher"
chmod +x "$OUT/Contents/MacOS/diskette-launcher"

GOOS=darwin GOARCH="$ARCH" go build -trimpath -ldflags "-X main.version=$VERSION" -o "$OUT/Contents/MacOS/diskette-bin" "$ROOT/cmd/diskette"

echo "Built $OUT (darwin/$ARCH, version $VERSION)"
echo "Install: cp -R \"$OUT\" /Applications/"
