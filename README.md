# Diskette

A cross-platform dual-pane terminal file manager, in the style of classic
Total Commander/Midnight Commander, built on the
[graphite](https://github.com/yeoblyv/graphite) TUI framework.

> Beta release (v0.1.0). Expect rough edges.

## Features

- Two navigable panes with an F-key action bar (Info, Rename, Find, Grep,
  Copy, Move, MkDir, Delete, Menu, Quit).
- Tabs per pane: file lists, embedded terminals, and remote connections,
  with pinned tabs restored on the next launch (fault-tolerant — this
  also happens on a signal-terminated process, not only a confirmed quit).
- Remote panes over SFTP and FTP/FTPS, with host-key TOFU verification
  for SFTP and reconnect/disconnect from a dedicated Network menu.
- Archiving: zip and unzip, generic across local and remote panes.
- File name search (F3) and grep-style file content search (F4) —
  literal or regular expression, recursive, case-sensitive, with a
  name-mask filter.
- Tagging: tag/untag, select all, deselect all, invert selection.
- An embedded terminal tab that can talk back to the running diskette
  instance (`diskette view`/`sync`/`tag`/`untag`/`select`) to drive the
  file panes from a shell.

## Install

Prebuilt binaries for macOS, Linux, and Windows (amd64/arm64/386) are
attached to each [release](https://github.com/yeoblyv/diskette/releases).
See [packaging/README.md](packaging/README.md) for platform-specific
install steps (the macOS `.app`, the Windows installer, the Linux
`install.sh`) and how those artifacts are built.

## Build from source

Requires Go 1.26+.

```
git clone https://github.com/yeoblyv/diskette.git
cd diskette
go build -o diskette ./cmd/diskette
```

## License

[MIT](LICENSE)
