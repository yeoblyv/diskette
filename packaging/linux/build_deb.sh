#!/bin/bash
# Builds a .deb package for the given architecture from an already-built
# binary in dist/. Run from the repo root:
#   packaging/linux/build_deb.sh [amd64|arm64] [version] [path-to-binary]
#
# version defaults to the nearest git tag (stripped of its leading "v"),
# same convention as build_app.sh. The binary defaults to
# dist/diskette-linux-<arch>, matching the release cross-compile layout.
set -euo pipefail

ARCH="${1:-amd64}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
VERSION="${2:-$(git -C "$ROOT" describe --tags --always --dirty | sed 's/^v//')}"
BIN="${3:-$ROOT/dist/diskette-linux-$ARCH}"

if [ ! -f "$BIN" ]; then
	echo "error: binary not found at $BIN (cross-compile it first, or pass its path)" >&2
	exit 1
fi

STAGE="$ROOT/dist/deb-stage-$ARCH"
OUT="$ROOT/dist/diskette_${VERSION}_${ARCH}.deb"

rm -rf "$STAGE"
mkdir -p "$STAGE/DEBIAN" \
	"$STAGE/usr/bin" \
	"$STAGE/usr/share/applications" \
	"$STAGE/usr/share/icons/hicolor/256x256/apps" \
	"$STAGE/usr/share/doc/diskette"

install -m 755 "$BIN" "$STAGE/usr/bin/diskette"
install -m 644 "$ROOT/packaging/linux/diskette.desktop" "$STAGE/usr/share/applications/diskette.desktop"
install -m 644 "$ROOT/packaging/icons/icon-256.png" "$STAGE/usr/share/icons/hicolor/256x256/apps/diskette.png"
install -m 644 "$ROOT/LICENSE" "$STAGE/usr/share/doc/diskette/copyright"

cat > "$STAGE/DEBIAN/control" <<EOF
Package: diskette
Version: $VERSION
Section: utils
Priority: optional
Architecture: $ARCH
Maintainer: Yehor Oblyvantsov <xineraman8@gmail.com>
Homepage: https://github.com/yeoblyv/diskette
Description: Cross-platform dual-pane terminal file manager
 A dual-pane terminal file manager in the style of classic Total
 Commander/Midnight Commander, with tabs, remote SFTP/FTP panes,
 archiving, and file search/grep, built on the graphite TUI framework.
EOF

dpkg-deb --build --root-owner-group "$STAGE" "$OUT"
rm -rf "$STAGE"

echo "Built $OUT"
