#!/bin/bash
# Assembles Diskette.app: compiles the real binary for the given GOARCH,
# wraps it with the Terminal-launcher script (see diskette-launcher),
# and drops in Info.plist/AppIcon.icns. Run from the repo root:
#   packaging/macos/build_app.sh [arm64|amd64]
set -euo pipefail

ARCH="${1:-arm64}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT="$ROOT/dist/Diskette.app"

rm -rf "$OUT"
mkdir -p "$OUT/Contents/MacOS" "$OUT/Contents/Resources"

cp "$ROOT/packaging/macos/Info.plist" "$OUT/Contents/Info.plist"
cp "$ROOT/packaging/icons/AppIcon.icns" "$OUT/Contents/Resources/AppIcon.icns"
cp "$ROOT/packaging/macos/diskette-launcher" "$OUT/Contents/MacOS/diskette-launcher"
chmod +x "$OUT/Contents/MacOS/diskette-launcher"

GOOS=darwin GOARCH="$ARCH" go build -o "$OUT/Contents/MacOS/diskette-bin" "$ROOT/cmd/diskette"

echo "Built $OUT (darwin/$ARCH)"
echo "Install: cp -R \"$OUT\" /Applications/"
