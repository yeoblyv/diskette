#!/bin/bash
# Installs Diskette for the current user (no root needed): the binary goes
# to ~/.local/bin, the icon and .desktop entry go to the standard XDG
# per-user data directories, so it shows up in the desktop's application
# launcher (GNOME Activities, KDE's launcher, etc.) with a real icon.
# Terminal=true in diskette.desktop tells the launcher to run it inside a
# terminal emulator, the same problem macOS's launcher script solves.
#
# Usage: packaging/linux/install.sh [path-to-diskette-binary]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${1:-$ROOT/../dist/diskette-linux-amd64}"

if [ ! -f "$BIN" ]; then
	echo "error: binary not found at $BIN (build it first, or pass its path)" >&2
	exit 1
fi

BIN_DIR="$HOME/.local/bin"
ICON_DIR="$HOME/.local/share/icons/hicolor/256x256/apps"
APPS_DIR="$HOME/.local/share/applications"

mkdir -p "$BIN_DIR" "$ICON_DIR" "$APPS_DIR"
install -m 755 "$BIN" "$BIN_DIR/diskette"
install -m 644 "$ROOT/linux/diskette.png" "$ICON_DIR/diskette.png"
install -m 644 "$ROOT/linux/diskette.desktop" "$APPS_DIR/diskette.desktop"

command -v update-desktop-database >/dev/null && update-desktop-database "$APPS_DIR" || true
command -v gtk-update-icon-cache >/dev/null && gtk-update-icon-cache "$HOME/.local/share/icons/hicolor" || true

echo "Installed diskette to $BIN_DIR/diskette"
echo "Make sure $BIN_DIR is on your PATH, then find \"Diskette\" in your application launcher."
