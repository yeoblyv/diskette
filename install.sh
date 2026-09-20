#!/bin/sh
# Downloads and installs the latest diskette release for this machine's
# OS/arch — no git clone, no Go toolchain, no root needed. Usage:
#
#   curl -fsSL https://raw.githubusercontent.com/yeoblyv/diskette/main/install.sh | sh
#
# Installs to $DISKETTE_INSTALL_DIR (default: $HOME/.local/bin), verifying
# the download against the release's published SHA256SUMS.txt.
set -eu

REPO="yeoblyv/diskette"
INSTALL_DIR="${DISKETTE_INSTALL_DIR:-$HOME/.local/bin}"

os=""
case "$(uname -s)" in
	Linux) os="linux" ;;
	Darwin) os="darwin" ;;
	*)
		echo "install.sh: unsupported OS '$(uname -s)' — see https://github.com/$REPO/releases for a manual download" >&2
		exit 1
		;;
esac

arch=""
case "$(uname -m)" in
	x86_64 | amd64) arch="amd64" ;;
	aarch64 | arm64) arch="arm64" ;;
	*)
		echo "install.sh: unsupported architecture '$(uname -m)' — see https://github.com/$REPO/releases for a manual download" >&2
		exit 1
		;;
esac

echo "install.sh: resolving the latest release..."
tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
if [ -z "$tag" ]; then
	echo "install.sh: could not resolve the latest release tag from the GitHub API" >&2
	exit 1
fi
version="${tag#v}"

asset="diskette-${tag}-${os}-${arch}"
base_url="https://github.com/$REPO/releases/download/$tag"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "install.sh: downloading $asset ($tag)..."
curl -fsSL -o "$tmp/$asset" "$base_url/$asset"
curl -fsSL -o "$tmp/SHA256SUMS.txt" "$base_url/SHA256SUMS.txt"

echo "install.sh: verifying checksum..."
(cd "$tmp" && grep " $asset\$" SHA256SUMS.txt | shasum -a 256 -c - 2>/dev/null || grep " $asset\$" SHA256SUMS.txt | sha256sum -c -)

mkdir -p "$INSTALL_DIR"
install -m 755 "$tmp/$asset" "$INSTALL_DIR/diskette"

echo "install.sh: installed diskette $version to $INSTALL_DIR/diskette"

case ":$PATH:" in
	*":$INSTALL_DIR:"*) ;;
	*)
		echo ""
		echo "$INSTALL_DIR is not on your PATH. Add this to your shell profile:"
		echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
		;;
esac
