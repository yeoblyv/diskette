#!/bin/bash
# Regenerates every icon file this repo ships, from
# internal/assets/diskette.gph (the same logo the About dialog embeds) —
# run this after that logo ever changes. macOS-only: relies on sips and
# iconutil, both Apple system tools.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ICONS="$ROOT/packaging/icons"

go run "$ROOT/cmd/gen-icon-png" -out "$ICONS/icon-1024.png"

for sz in 16 32 48 64 128 256; do
	sips -z "$sz" "$sz" "$ICONS/icon-1024.png" --out "$ICONS/icon-$sz.png" >/dev/null
done

rm -rf "$ICONS/AppIcon.iconset"
mkdir -p "$ICONS/AppIcon.iconset"
for sz in 16 32 128 256 512; do
	sips -z "$sz" "$sz" "$ICONS/icon-1024.png" --out "$ICONS/AppIcon.iconset/icon_${sz}x${sz}.png" >/dev/null
	sz2=$((sz * 2))
	sips -z "$sz2" "$sz2" "$ICONS/icon-1024.png" --out "$ICONS/AppIcon.iconset/icon_${sz}x${sz}@2x.png" >/dev/null
done
iconutil -c icns "$ICONS/AppIcon.iconset" -o "$ICONS/AppIcon.icns"
rm -rf "$ICONS/AppIcon.iconset"

cp "$ICONS/icon-256.png" "$ROOT/packaging/linux/diskette.png"

echo "Regenerated:"
echo "  $ICONS/icon-1024.png (source)"
echo "  $ICONS/icon-{16,32,48,64,128,256}.png (Windows .ico sizes)"
echo "  $ICONS/AppIcon.icns (macOS)"
echo
echo "Now re-embed the Windows icon and rebuild the .app bundle:"
echo "  go-winres make --in packaging/windows/winres.json --arch amd64,386 --out cmd/diskette/rsrc"
echo "  packaging/macos/build_app.sh"
