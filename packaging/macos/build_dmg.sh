#!/bin/bash
# Wraps dist/Diskette.app (build it first with build_app.sh) into a
# drag-to-Applications .dmg for easy handoff. Run from the repo root:
#   packaging/macos/build_dmg.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
APP="$ROOT/dist/Diskette.app"
STAGE="$ROOT/dist/dmg-stage"
DMG="$ROOT/dist/Diskette.dmg"

if [ ! -d "$APP" ]; then
	echo "error: $APP not found — run build_app.sh first" >&2
	exit 1
fi

rm -rf "$STAGE" "$DMG"
mkdir -p "$STAGE"
cp -R "$APP" "$STAGE/"
ln -s /Applications "$STAGE/Applications"

hdiutil create -volname Diskette -srcfolder "$STAGE" -ov -format UDZO "$DMG"
rm -rf "$STAGE"

echo "Built $DMG"
