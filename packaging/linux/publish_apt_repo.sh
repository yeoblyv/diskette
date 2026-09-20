#!/bin/bash
# Publishes dist/apt-repo (build it first with build_apt_repo.sh) to the
# gh-pages branch under /apt, which is what GitHub Pages actually serves
# — keeping every .deb this ever produces out of main's history, the same
# "build output is never committed" rule dist/ itself follows. Run from
# the repo root:
#   packaging/linux/publish_apt_repo.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO_DIR="$ROOT/dist/apt-repo"

if [ ! -d "$REPO_DIR" ]; then
	echo "error: $REPO_DIR not found — run build_apt_repo.sh first" >&2
	exit 1
fi

WORKTREE="$ROOT/dist/gh-pages-worktree"
rm -rf "$WORKTREE"

if git -C "$ROOT" show-ref --quiet refs/remotes/origin/gh-pages; then
	git -C "$ROOT" worktree add -B gh-pages "$WORKTREE" origin/gh-pages
else
	git -C "$ROOT" worktree add --orphan -B gh-pages "$WORKTREE"
fi

rm -rf "$WORKTREE/apt"
cp -R "$REPO_DIR" "$WORKTREE/apt"

git -C "$WORKTREE" add apt
if git -C "$WORKTREE" diff --cached --quiet; then
	echo "Nothing changed — apt repo already up to date."
else
	git -C "$WORKTREE" commit -m "chore: publish apt repo ($(date -u '+%Y-%m-%d %H:%M UTC'))"
	git -C "$WORKTREE" push origin gh-pages
	echo "Published to gh-pages."
fi

git -C "$ROOT" worktree remove --force "$WORKTREE"
