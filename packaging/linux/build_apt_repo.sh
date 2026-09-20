#!/bin/bash
# Assembles a flat APT repository under dist/apt-repo from the .deb files
# in dist/ (build those first with build_deb.sh). Run from the repo root:
#   packaging/linux/build_apt_repo.sh
#
# dist/ is never committed (build output — see .gitignore), so publishing
# the result is a separate step: publish_apt_repo.sh pushes this
# directory's contents to the orphan gh-pages branch, which is what
# GitHub Pages actually serves.
#
# There is no GPG key to sign the repo with yet, so users add it with
# [trusted=yes] — see the README this script writes into the repo dir.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO_DIR="$ROOT/dist/apt-repo"
POOL_DIR="$REPO_DIR/pool"

shopt -s nullglob
debs=("$ROOT"/dist/*.deb)
if [ ${#debs[@]} -eq 0 ]; then
	echo "error: no .deb files in dist/ — run build_deb.sh first" >&2
	exit 1
fi

rm -rf "$REPO_DIR"
mkdir -p "$POOL_DIR"
cp "${debs[@]}" "$POOL_DIR/"

for arch in amd64 arm64; do
	out_dir="$REPO_DIR/dists/stable/main/binary-$arch"
	mkdir -p "$out_dir"
	(cd "$REPO_DIR" && dpkg-scanpackages --arch "$arch" pool /dev/null) > "$out_dir/Packages"
	gzip -9c "$out_dir/Packages" > "$out_dir/Packages.gz"
done

cat > "$REPO_DIR/dists/stable/Release" <<EOF
Origin: diskette
Label: diskette
Suite: stable
Codename: stable
Architectures: amd64 arm64
Components: main
Description: APT repository for diskette (unsigned — see README.md)
Date: $(date -u '+%a, %d %b %Y %H:%M:%S UTC')
EOF

cat > "$REPO_DIR/README.md" <<'EOF'
# diskette APT repository

Unsigned (no GPG key yet) — install with `[trusted=yes]`:

```bash
echo "deb [trusted=yes] https://yeoblyv.github.io/diskette/apt stable main" | sudo tee /etc/apt/sources.list.d/diskette.list
sudo apt update
sudo apt install diskette
```

`[trusted=yes]` skips APT's signature check for this repo specifically —
it does not affect any other configured repository. This is a known,
temporary gap: prefer the [curl installer](https://github.com/yeoblyv/diskette/blob/main/install.sh)
or a [direct release download](https://github.com/yeoblyv/diskette/releases)
until the repo is signed.
EOF

echo "Built $REPO_DIR"
