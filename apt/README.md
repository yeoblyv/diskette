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
