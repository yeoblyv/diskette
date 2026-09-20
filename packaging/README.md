# Packaging

Diskette is a terminal UI, which shapes how each platform launches it:
a plain binary has no icon Explorer/Finder/a launcher can show, and no
attached terminal when launched from a GUI (Spotlight, Start menu, an
app launcher) rather than a shell.

## Icons

`packaging/icons/generate.sh` renders `internal/assets/diskette.gph` (the
same logo the About dialog embeds) into every icon file this repo ships:
`icon-1024.png` (source), `icon-{16,32,48,64,128,256}.png` (Windows `.ico`
sizes), and `AppIcon.icns` (macOS). Run it after the logo ever changes,
macOS only (it shells out to `sips`/`iconutil`), then follow the two
commands it prints to re-embed the Windows icon and rebuild the `.app`.

## macOS

`packaging/macos/build_app.sh [arm64|amd64]` assembles `dist/Diskette.app`:
`Info.plist`, `AppIcon.icns`, and a launcher wrapper. A `.app` bundle has
no terminal attached when Finder/Spotlight open it, so
`Contents/MacOS/diskette-launcher` (the bundle's actual
`CFBundleExecutable`) opens Terminal.app and runs the real binary
(`diskette-bin`) inside it via `osascript`, rather than running the TUI
directly with nowhere to draw.

Install by copying to `/Applications`, or build a drag-and-drop image with
`packaging/macos/build_dmg.sh` (run `build_app.sh` first). Neither is
code-signed, so Gatekeeper will refuse a first double-click launch —
right-click → Open once bypasses that; there is no local Apple Developer
certificate to sign with here.

## Windows

Icon embedding happens at build time via
[go-winres](https://github.com/tc-hib/go-winres)
(`go install github.com/tc-hib/go-winres@latest`), reading
`packaging/windows/winres.json` and writing `rsrc_windows_{386,amd64}.syso`
straight into `cmd/diskette/` — Go's toolchain links any `.syso` file it
finds there automatically, so a plain `GOOS=windows go build` already
produces an iconed `.exe` with no extra build step. These `.syso` files
are checked into the repo (small, deterministic from the icon source) so
that stays true without regenerating them first.

Regenerate after the icon changes:

```
go-winres make --in packaging/windows/winres.json --arch amd64,386 --out cmd/diskette/rsrc
```

Unlike a macOS `.app`, a console-subsystem `.exe` opens its own console
window automatically when launched, so no wrapper script is needed —
`packaging/windows/install.ps1` just copies the exe to
`%LOCALAPPDATA%\Diskette` and creates a Start Menu shortcut (the shortcut
inherits the exe's own embedded icon).

`packaging/windows/installer.nsi` is a real NSIS installer script
(Program Files, Start Menu + Desktop shortcuts, an uninstaller, an
Add/Remove Programs entry) for anyone who wants that instead of the
plain PowerShell script — **currently unbuildable from this machine**:
`makensis` (installed via `brew install makensis`, and rebuilt
`--build-from-source` to rule out a bad bottle) crashes with
`std::bad_alloc` while writing output, on a two-line test script with no
Diskette-specific content — an upstream NSIS/macOS compatibility issue,
not a bug in the script. It should build fine from an actual Windows
machine or one with a working `makensis`; the script itself hasn't been
run successfully end-to-end.

## Linux

`packaging/linux/install.sh [path-to-binary]` installs per-user (no root):
the binary to `~/.local/bin`, the icon to
`~/.local/share/icons/hicolor/256x256/apps`, and `diskette.desktop` to
`~/.local/share/applications` — the standard XDG layout every major
desktop environment's launcher (GNOME Activities, KDE, etc.) picks up
from automatically. The desktop entry's `Terminal=true` is Linux's
equivalent of macOS's launcher wrapper: it tells the launcher to run the
command inside a terminal emulator instead of headlessly.

For a headless server rather than a desktop, the repo root's
[`install.sh`](../install.sh) is simpler: it downloads the latest
release's binary for the current OS/arch straight from GitHub and drops
it on `PATH`, no packaging or desktop integration involved.

### .deb package and APT repo

`packaging/linux/build_deb.sh [amd64|arm64] [version] [path-to-binary]`
assembles `dist/diskette_<version>_<arch>.deb` (binary, icon, `.desktop`
entry, `/usr/share/doc/diskette/copyright`) from an already
cross-compiled binary — needs `dpkg-deb` (preinstalled on any Debian/
Ubuntu box; `brew install dpkg` on macOS).

`packaging/linux/build_apt_repo.sh` then assembles a flat APT repository
under `dist/apt-repo` from every `.deb` in `dist/` — needs
`dpkg-scanpackages` (same `dpkg` package). `packaging/linux/publish_apt_repo.sh`
pushes that directory to the orphan `gh-pages` branch under `/apt`, which
GitHub Pages serves — `dist/` itself is never committed to `main` (build
output, see `.gitignore`), so the repo's actual `.deb` history lives only
on `gh-pages`, not in `main`'s. The result is installable as:

```bash
echo "deb [trusted=yes] https://yeoblyv.github.io/diskette/apt stable main" | sudo tee /etc/apt/sources.list.d/diskette.list
sudo apt update && sudo apt install diskette
```

`[trusted=yes]` is required because the repo isn't GPG-signed yet — see
the `README.md` `build_apt_repo.sh` writes alongside it.
