# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2026-09-20

### Added

- Full internationalization: English, Ukrainian, Russian, and Dutch, switchable from Help > Language.
- `diskette --version` / `-v` prints the build's version and exits.

### Fixed

- Two severe Copy/Move data-loss bugs: copying or moving a directory onto itself or into its own subdirectory could destroy its contents; both are now refused or handled safely, with a full regression test suite around Copy/Move.
- The gutter Copy/Move buttons always treated the left pane as the source, even when the right pane was focused and selected — a mouse click could silently no-op or act on the wrong pane.
- A stray cursor block could remain drawn under a newly opened modal dialog when a focused input box lost focus without being told.
- Dropdown list contrast: a ComboBox's closed state and its selected dropdown row could be unreadable against a bright theme accent color.
- The FilePane's Attr column could collide with the scrollbar, corrupting the last character of a file's permission string.
- Manage Tabs' Close buttons could be truncated in translated locales.
- Running `diskette` from inside one of its own embedded terminal tabs now shows a help listing instead of trying to nest a second full-screen instance.

## [0.1.0] - 2026-09-11

Initial public beta.

### Added

- Dual-pane file browsing with the classic F-key action bar (Info, Rename, Find, Grep, Copy, Move, MkDir, Delete, Quit).
- Tabs per pane: file lists, embedded terminals, and remote connections, with pinned tabs restored on next launch, including after a signal-terminated process.
- Remote panes over SFTP and FTP/FTPS, with SFTP host-key TOFU verification.
- Zip/unzip archiving, generic across local and remote panes.
- File name search (F3) and grep-style content search (F4): literal or regex, recursive, case-sensitive, name-mask filterable.
- Tagging: tag/untag, select all, deselect all, invert selection.
- An embedded terminal that can drive the file panes via `diskette view`/`sync`/`tag`/`untag`/`select`.

[0.2.0]: https://github.com/yeoblyv/diskette/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/yeoblyv/diskette/releases/tag/v0.1.0
