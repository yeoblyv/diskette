// Command diskette is a dual-pane file manager built on the graphite TUI
// framework, in the style of classic Total Commander/Midnight Commander:
// two navigable panes, an F-key action bar, and local file operations.
// Later phases add remote (SFTP) panes.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	Graphite "github.com/yeoblyv/graphite"

	"github.com/yeoblyv/diskette/internal/archiveengine"
	"github.com/yeoblyv/diskette/internal/assets"
	"github.com/yeoblyv/diskette/internal/copyengine"
	"github.com/yeoblyv/diskette/internal/diskspace"
	"github.com/yeoblyv/diskette/internal/fileinfo"
	"github.com/yeoblyv/diskette/internal/filepane"
	"github.com/yeoblyv/diskette/internal/fkeybar"
	"github.com/yeoblyv/diskette/internal/grep"
	"github.com/yeoblyv/diskette/internal/locales"
	"github.com/yeoblyv/diskette/internal/openwith"
	"github.com/yeoblyv/diskette/internal/roots"
	"github.com/yeoblyv/diskette/internal/search"
	"github.com/yeoblyv/diskette/internal/tabs"
	"github.com/yeoblyv/diskette/internal/theme"
	"github.com/yeoblyv/diskette/internal/vfs"
)

// navAccent is the oceanic accent color shared by the navigation row's
// buttons and the copy/move gutter buttons, so both stand out against the
// chrome background instead of blending into it — distinct from Primary
// (pane focus/cursor) and Warning (the menu strip), which already carry
// their own meaning.
var navAccent = Graphite.Hex("#5DE4FF")

// main dispatches to the CLI (a `diskette view`/`sync`/`tag`/`untag`/
// `select` invoked from inside one of this program's own Terminal tabs —
// see cli.go) before ever touching the TUI, so that second invocation of
// this same binary exits immediately instead of trying to open a nested
// full-screen instance inside its own terminal.
func main() {
	if code, handled := runCLI(os.Args[1:]); handled {
		os.Exit(code)
	}
	runTUI()
}

func runTUI() {
	start, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "diskette:", err)
		os.Exit(1)
	}

	app := Graphite.NewApplication()
	app.SetTheme(theme.Diskette())
	setupLocales(app)
	win := Graphite.NewFullscreenWindow()

	fs := vfs.LocalFS{}

	leftTabs := newPaneTabs(app, fs)
	rightTabs := newPaneTabs(app, fs)

	activeGroup := func() *paneTabs {
		if rightTabs.HasFocus() {
			return rightTabs
		}
		return leftTabs
	}
	otherGroup := func(g *paneTabs) *paneTabs {
		if g == leftTabs {
			return rightTabs
		}
		return leftTabs
	}

	// The IPC server needs otherGroup to resolve "the other pane" for an
	// incoming request, so it starts here rather than any earlier — and
	// restoreSavedTabs (which can itself spawn a pinned Terminal tab) has
	// to wait for it, so that tab gets working DISKETTE_IPC/TERMINAL_ID
	// env vars too rather than ones pointing nowhere. A failure here
	// (the loopback interface unavailable, astonishingly rare) disables
	// `diskette view`/`sync`/`tag` rather than the whole program — every
	// Terminal tab just won't have those env vars set, which cliView/
	// cliSync/cliTagging already treat as an ordinary, reported error.
	ipcReg := newTerminalRegistry()
	ipcAddr, ipcErr := startIPCServer(app, ipcReg, otherGroup)
	if ipcErr != nil {
		fmt.Fprintln(os.Stderr, "diskette: terminal integration (view/sync/tag) unavailable:", ipcErr)
	}
	leftTabs.ipcTermRegistry, leftTabs.ipcAddr = ipcReg, ipcAddr
	rightTabs.ipcTermRegistry, rightTabs.ipcAddr = ipcReg, ipcAddr

	restoreSavedTabs(leftTabs, rightTabs, start)
	// withActiveFilePane wraps an action that only makes sense against a
	// FileList tab (rename, copy, delete, ...) so every F-key/menu
	// callsite doesn't have to repeat the "is the active tab even a file
	// list" guard — if the focused pane's active tab is a Terminal
	// instead, the action is silently a no-op rather than acting on stale
	// or wrong data.
	withActiveFilePane := func(fn func(fp *filepane.FilePane)) func() {
		return func() {
			if fp, ok := activeGroup().ActiveFilePane(); ok {
				fn(fp)
			}
		}
	}
	withActivePanes := func(fn func(src, dst *filepane.FilePane)) func() {
		return func() {
			src, ok1 := activeGroup().ActiveFilePane()
			dst, ok2 := otherGroup(activeGroup()).ActiveFilePane()
			if ok1 && ok2 {
				fn(src, dst)
			}
		}
	}

	fKeyBar := newFKeyBar(app, rightTabs, withActiveFilePane, withActivePanes)

	refreshStatus := func() {
		if fp, ok := activeGroup().ActiveFilePane(); ok {
			if q := fp.SearchQuery(); q != "" {
				fKeyBar.Status = app.T(locales.KeyStatusSearch, q)
				return
			}
		}
		fKeyBar.Status = ""
	}
	leftTabs.onSearchChanged = refreshStatus
	rightTabs.onSearchChanged = refreshStatus
	refreshStatus()

	mainRow := Graphite.NewFlex(0, 2, 0, -3, Graphite.FlexRow)
	mainRow.Gap = 1
	mainRow.AddChild(leftTabs, 1)
	mainRow.AddChild(newActionGutter(app, leftTabs, rightTabs), 0)
	mainRow.AddChild(rightTabs, 1)

	diskBar := newDiskSpaceBar(app, func() statusBarState {
		group := leftTabs
		if rightTabs.HasFocus() {
			group = rightTabs
		}
		fp, ok := group.ActiveFilePane()
		if !ok {
			return statusBarState{}
		}
		count, size, hasTagged := fp.TaggedSummary()
		state := statusBarState{
			taggedCount: count, taggedSize: size, hasTagged: hasTagged,
			remoteLabel: group.ActiveRemoteLabel(),
		}
		if _, isLocal := fp.FS.(vfs.LocalFS); isLocal {
			usage, err := diskspace.Query(fp.Path())
			state.usage = diskUsageCache{usage: usage, ok: err == nil}
		}
		return state
	})

	win.AddWidget(mainRow)
	win.AddWidget(diskBar)
	win.AddWidget(fKeyBar)
	// menu is added last so its open dropdown wins Window's hit-test
	// priority over the panes it visually overlaps — see newMenuStrip.
	win.AddWidget(newMenuStrip(app, leftTabs, rightTabs, activeGroup, withActiveFilePane, withActivePanes))

	fKeyHandler := func(fp *filepane.FilePane) func(Graphite.KeyCode) {
		return func(key Graphite.KeyCode) {
			switch key {
			case Graphite.KeyF1:
				showFileInfo(app, fp)
			case Graphite.KeyF2:
				doRename(app, fp)
			case Graphite.KeyF3:
				showFindFiles(app, fp)
			case Graphite.KeyF4:
				showGrepSearch(app, fp)
			case Graphite.KeyF5:
				if dst, ok := otherGroup(activeGroup()).ActiveFilePane(); ok {
					doCopyOrMove(app, fp, dst, false)
				}
			case Graphite.KeyF6:
				if dst, ok := otherGroup(activeGroup()).ActiveFilePane(); ok {
					doCopyOrMove(app, fp, dst, true)
				}
			case Graphite.KeyF7:
				doMkdir(app, fp)
			case Graphite.KeyF8:
				doDelete(app, fp)
			case Graphite.KeyF10:
				requestQuit(app, leftTabs, rightTabs)
			}
		}
	}
	leftTabs.onFileListFKey = fKeyHandler
	rightTabs.onFileListFKey = fKeyHandler

	app.SetOnQuitRequested(func() { requestQuit(app, leftTabs, rightTabs) })
	installSignalSaveHandler(app, leftTabs, rightTabs)
	app.SetWindow(win)
	app.Run()
}

// setupLocales registers diskette's own translation catalogs (see
// internal/locales) for every language it ships, and sets the current
// one: whatever was chosen last time via the Help menu's Language
// submenu (persisted in tabs.json, see saveTabs), or failing that,
// whatever detectLocale can infer from the OS environment. Must run
// before any widget that draws translated text is built — the menu
// strip, F-key bar, and every FilePane all read Application.T (directly
// or via FilePane.Translate) as they're constructed.
func setupLocales(app *Graphite.Application) {
	app.SetTranslations(Graphite.LocaleEnglish, locales.English)
	app.SetTranslations(Graphite.LocaleUkrainian, locales.Ukrainian)
	app.SetTranslations(Graphite.LocaleRussian, locales.Russian)
	app.SetTranslations(Graphite.LocaleDutch, locales.Dutch)

	state, _ := tabs.Load() // a missing/unreadable file just means "nothing saved yet"
	app.SetLocale(detectLocale(state.Locale))
}

// detectLocale returns saved as a Locale if it's non-empty (a language
// explicitly chosen before, see setupLocales), otherwise falls back to
// the OS environment: the first of $LC_ALL/$LC_MESSAGES/$LANG (the
// standard POSIX precedence order) whose language subtag names one of
// diskette's translated locales, or LocaleEnglish if none does or none
// of those variables is set. graphite itself never reads the
// environment for this (see docs/i18n.md's "No locale auto-detection"),
// so this is diskette's own job, done once at startup.
func detectLocale(saved string) Graphite.Locale {
	if saved != "" {
		return Graphite.Locale(saved)
	}
	for _, envVar := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := os.Getenv(envVar)
		if v == "" {
			continue
		}
		lang, _, _ := strings.Cut(v, "_")
		lang, _, _ = strings.Cut(lang, ".")
		switch Graphite.Locale(lang) {
		case Graphite.LocaleUkrainian, Graphite.LocaleRussian, Graphite.LocaleDutch:
			return Graphite.Locale(lang)
		}
	}
	return Graphite.LocaleEnglish
}

// installSignalSaveHandler makes SIGINT/SIGTERM/SIGHUP save pinned tabs
// and clean up before the process actually dies, the same way a
// confirmed F10/Escape quit already does — closing the terminal window
// diskette is running in, a plain Ctrl+C, or a `kill` all bypass
// requestQuit's confirmation entirely, and previously bypassed saving
// pinned tabs right along with it: nothing pinned in that session ever
// made it to disk, no matter how carefully it was pinned, because the
// save step itself never ran. Signal delivery in Go doesn't wait for the
// main goroutine to be idle (it's handled on a runtime-owned thread), so
// this fires promptly even while the main loop is blocked reading
// terminal input — saveAndQuit itself must still run on the main loop via
// app.Invoke, since it touches the same tab/widget state the main loop
// reads and writes without any locking of its own.
func installSignalSaveHandler(app *Graphite.Application, left, right *paneTabs) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sig
		app.Invoke(func() { saveAndQuit(app, left, right) })
	}()
}

// defaultShell picks the interactive shell a new Terminal tab spawns:
// $SHELL on Unix (falling back to /bin/sh if unset), %COMSPEC% on
// Windows (falling back to cmd.exe) — the same fallback chain a real
// terminal emulator's own "open a new tab" uses.
func defaultShell() string {
	if runtime.GOOS == "windows" {
		if comspec := os.Getenv("COMSPEC"); comspec != "" {
			return comspec
		}
		return "cmd.exe"
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/sh"
}

// newNavButton creates a small, non-focusable Button. It is deliberately
// not focusable (IsFocusable is forced back to false right after
// construction): only the active tab's own content widget in each pane
// stays focusable, so Tab keeps switching directly between the two panes
// — see the project's architecture notes on this. Clicking it still works
// regardless of focusability, since Window delivers a mouse click to
// whatever it hit-tests under the pointer independently of the Tab order.
func newNavButton(label string, onClick func()) *Graphite.Button {
	btn := Graphite.NewButton(0, 0, label, Graphite.BtnDefault, onClick)
	btn.IsFocusable = false
	btn.BgColor = navAccent
	return btn
}

// openFile hands path to the OS's associated default application — the
// same effect as double-clicking it in Finder/Explorer — reporting a
// failure (e.g. no handler registered for the file's type) the same way
// every other file operation reports one.
func openFile(app *Graphite.Application, path string) {
	if err := openwith.Open(path); err != nil {
		app.ShowMessage(app.T(locales.KeyErrorTitle), err.Error(), Graphite.BtnDanger)
	}
}

// newNavRow builds one FileList tab's navigation strip — Back/Forward, a
// root/drive picker, and the clickable current-path bar — as a Flex row
// so the path bar always fills exactly the space the fixed-width buttons
// leave, at any terminal width. There is deliberately no "up one level"
// button (the ".." row already at the top of every listing does that) and
// no manual Reload button — FilePane now auto-refreshes its own listing
// on a timer (see internal/filepane's autoRefresh).
func newNavRow(app *Graphite.Application, fp *filepane.FilePane) *Graphite.Flex {
	row := Graphite.NewFlex(0, 0, 0, 1, Graphite.FlexRow)
	row.Gap = 1
	row.AddChild(newNavButton("<", func() { fp.Back() }), 0)
	row.AddChild(newNavButton(">", func() { fp.Forward() }), 0)
	row.AddChild(newNavButton(app.T(locales.KeyNavRoot), func() {
		if _, ok := fp.FS.(vfs.LocalFS); ok {
			promptChooseRoot(app, fp)
		} else {
			// A remote server has exactly one root — no drives/volumes
			// picker (internal/roots) makes sense for it, and that
			// picker is unconditionally local anyway.
			fp.SetPath("/")
		}
	}), 0)
	row.AddChild(newPathBar(app, fp), 1)
	return row
}

// tabContent is what one tab actually displays: for a FileList tab,
// filePane is the raw *filepane.FilePane (for everything that needs to
// act on it directly — F-keys, the menu, the gutter) and display is that
// FilePane wrapped in its own nav-row Flex; for a Terminal tab, terminal
// and display are the same bare *Graphite.Terminal. Kept as two named
// fields rather than a type switch on display everywhere a caller needs
// the concrete widget back. id is only set for a Terminal tab — its
// terminalRegistry key (see ipc.go), needed so CloseTabAt can unregister
// it. remote is set only for a Remote tab (still a FilePane under
// filePane — Remote is "a FileList backed by a different vfs.FileSystem",
// not a different display) — its SFTP session and the metadata needed to
// persist/reconnect it.
type tabContent struct {
	filePane *filepane.FilePane
	terminal *Graphite.Terminal
	display  Graphite.Widget
	id       string
	remote   *connectedRemote
}

// remoteFS is what a Remote tab's backing connection must offer beyond
// plain vfs.FileSystem: something to call when the tab closes or the app
// quits. Both internal/sftpfs.SFTPFS and internal/ftpfs.FTPFS already
// satisfy this with no changes to either package — it exists here purely
// so connectedRemote and AddRemote don't need to know or care which
// protocol they're holding.
type remoteFS interface {
	vfs.FileSystem
	Close() error
}

// connectedRemote is a Remote tab's live SFTP/FTP session plus the
// connection metadata pinnedOf persists (see internal/tabs.SavedRemote) —
// everything needed to show it in the status bar, close it when the tab
// closes or the app quits, and offer a pre-filled reconnect on restart,
// but never its password/passphrase.
type connectedRemote struct {
	label string // "user@host", for the tab name and status bar
	fs    remoteFS
	meta  tabs.SavedRemote
}

// focusable returns the one widget within this content that's actually
// focusable — the bare FilePane or Terminal, never the wrapping Flex a
// FileList tab draws its nav row in.
func (c tabContent) focusable() Graphite.Widget {
	if c.filePane != nil {
		return c.filePane
	}
	return c.terminal
}

// newFileListContent builds a fresh FileList tab at path: its own
// FilePane (independent cursor, tags, and back/forward history from every
// other tab) plus its nav row, wired the same way every FileList tab
// always has been.
func newFileListContent(app *Graphite.Application, fs vfs.FileSystem, path string) tabContent {
	fp := filepane.New(0, 0, 0, 0, fs, path)
	fp.Translate = app.T
	fp.OnOpenFile = func(p string) {
		if _, ok := fs.(vfs.LocalFS); !ok {
			app.ShowMessage(app.T(locales.KeyErrorTitle), app.T(locales.KeyErrRemoteEditUnsupported), Graphite.BtnDanger)
			return
		}
		openFile(app, p)
	}

	col := Graphite.NewFlex(0, 0, 0, 0, Graphite.FlexColumn)
	col.AddChild(newNavRow(app, fp), 0)
	col.AddChild(fp, 1)

	return tabContent{filePane: fp, display: col}
}

// newTerminalContent spawns a fresh shell in a new Terminal tab, with
// DISKETTE_IPC/DISKETTE_TERMINAL_ID/DISKETTE_SHELL_KIND set in its
// environment first (see cli.go/ipc.go) so `diskette view`/`sync`/`tag`
// run inside it can reach back into this diskette instance. Setting
// os.Setenv on the whole process rather than passing a per-child
// environment works because every Terminal spawn — here and everywhere
// else this is called from — happens on the main goroutine, so there's
// no concurrent access to race; startPTY (graphite's pty.go) captures
// os.Environ() synchronously inside this same call, on both Unix
// (appended explicitly) and Windows (CreateProcess's nil environment
// means "inherit the caller's").
func newTerminalContent(app *Graphite.Application, ipcAddr string) (tabContent, error) {
	shell := defaultShell()
	id := newTerminalID()

	os.Setenv("DISKETTE_IPC", ipcAddr)
	os.Setenv("DISKETTE_TERMINAL_ID", id)
	os.Setenv("DISKETTE_LOCALE", string(app.Locale()))
	kind := shellKindOf(shell)
	os.Setenv("DISKETTE_SHELL_KIND", kind)
	if shimDir, err := cliShimDir(); err == nil {
		os.Setenv("PATH", pathWithSelfDir(os.Getenv("PATH"), shimDir))
	}
	args := configureAutoSync(kind)

	term, err := Graphite.NewTerminal(app, 0, 0, 0, 0, shell, args)
	if err != nil {
		return tabContent{}, err
	}
	return tabContent{terminal: term, display: term, id: id}, nil
}

// configureAutoSync makes a fresh Terminal tab sync live from the moment
// it opens, with no `eval "$(diskette sync on)"` for the user to remember
// to type themselves. An earlier version of this ran that eval in a
// throwaway `shell -c '...'` and then `exec`'d a real interactive shell
// in its place — which doesn't work: exec replaces the whole running
// process image, and a shell's functions and hook arrays (chpwd_functions
// and friends) live in that image, not in the environment, so they don't
// survive the swap. The hook only sticks if it's installed as part of the
// same shell process the user ends up typing into, which means hooking
// into that shell's own startup-file mechanism instead:
//
//   - zsh: point ZDOTDIR at a directory holding only a .zshenv that
//     restores the real ZDOTDIR before doing anything else — zsh re-reads
//     $ZDOTDIR before each later startup file (.zprofile/.zshrc/.zlogin),
//     so those still load from the real location — sources that real
//     .zshenv if there is one, then installs the hook.
//   - bash: --rcfile points at a small file that sources the user's own
//     ~/.bashrc (bash skips its normal rc lookup once --rcfile is given)
//     before installing the hook.
//
// Both shim files are singletons (zshSyncDotDir/bashSyncRCFile), reused
// by every Terminal tab, so opening several doesn't create one per tab —
// and, for zsh, so a later tab doesn't capture an already-overridden
// ZDOTDIR as if it were the user's real one.
func configureAutoSync(kind string) []string {
	switch kind {
	case "zsh":
		if dir, err := zshSyncDotDir(); err == nil {
			os.Setenv("ZDOTDIR", dir)
		}
		return nil
	case "bash":
		if rcfile, err := bashSyncRCFile(); err == nil {
			return []string{"--rcfile", rcfile, "-i"}
		}
		return nil
	default:
		return nil
	}
}

// zshSyncShim caches the one .zshenv shim directory this process creates
// (see zshSyncDotDir), computed from the real ZDOTDIR/HOME before it's
// ever overridden.
var zshSyncShim struct {
	dir string
	err error
	set bool
}

func zshSyncDotDir() (string, error) {
	if zshSyncShim.set {
		return zshSyncShim.dir, zshSyncShim.err
	}
	zshSyncShim.set = true

	orig := os.Getenv("ZDOTDIR")
	if orig == "" {
		orig = os.Getenv("HOME")
	}
	dir, err := os.MkdirTemp("", "diskette-zdotdir-*")
	if err != nil {
		zshSyncShim.err = err
		return "", err
	}
	content := fmt.Sprintf(
		"export ZDOTDIR=%q\n[ -f \"$ZDOTDIR/.zshenv\" ] && source \"$ZDOTDIR/.zshenv\"\neval \"$(diskette sync on)\"\n",
		orig,
	)
	if err := os.WriteFile(filepath.Join(dir, ".zshenv"), []byte(content), 0o644); err != nil {
		os.RemoveAll(dir)
		zshSyncShim.err = err
		return "", err
	}
	zshSyncShim.dir = dir
	return dir, nil
}

// bashSyncShim caches the one --rcfile shim this process creates (see
// bashSyncRCFile), reused by every bash Terminal tab.
var bashSyncShim struct {
	path string
	err  error
	set  bool
}

func bashSyncRCFile() (string, error) {
	if bashSyncShim.set {
		return bashSyncShim.path, bashSyncShim.err
	}
	bashSyncShim.set = true

	dir, err := os.MkdirTemp("", "diskette-bashrc-*")
	if err != nil {
		bashSyncShim.err = err
		return "", err
	}
	const content = "[ -f \"$HOME/.bashrc\" ] && source \"$HOME/.bashrc\"\neval \"$(diskette sync on)\"\n"
	rcfile := filepath.Join(dir, "rc.sh")
	if err := os.WriteFile(rcfile, []byte(content), 0o644); err != nil {
		os.RemoveAll(dir)
		bashSyncShim.err = err
		return "", err
	}
	bashSyncShim.path = rcfile
	return rcfile, nil
}

// cliShimName is what a Terminal tab's shell needs to find on PATH to run
// the view/sync/tag/untag/select commands (cli.go) — the running binary's
// own name (e.g. "diskette-darwin-arm64", one of dist/'s per-platform
// names) isn't good enough, since PATH lookup matches by exact filename.
func cliShimName() string {
	if runtime.GOOS == "windows" {
		return "diskette.exe"
	}
	return "diskette"
}

// cliShim caches the one shim directory this process creates (see
// cliShimDir), so every Terminal tab shares it instead of each spawning
// its own.
var cliShim struct {
	dir string
	err error
	set bool
}

// cliShimDir lazily builds a small directory containing a single file
// literally named cliShimName() that resolves back to this running
// binary, so a spawned shell can always find `diskette` on PATH — via a
// symlink where possible, falling back to a hard link and finally a full
// copy for filesystems or platforms that support neither (a plain copy
// still works even though it won't reflect a binary replaced on disk
// after diskette started, which no shim scheme can do without re-running
// the parent process anyway).
func cliShimDir() (string, error) {
	if cliShim.set {
		return cliShim.dir, cliShim.err
	}
	cliShim.set = true

	exe, err := os.Executable()
	if err != nil {
		cliShim.err = err
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	dir, err := os.MkdirTemp("", "diskette-cli-*")
	if err != nil {
		cliShim.err = err
		return "", err
	}
	link := filepath.Join(dir, cliShimName())

	if err := os.Symlink(exe, link); err != nil {
		if err := os.Link(exe, link); err != nil {
			if err := copyFile(exe, link); err != nil {
				os.RemoveAll(dir)
				cliShim.err = err
				return "", err
			}
		}
	}

	cliShim.dir = dir
	return dir, nil
}

// copyFile is cliShimDir's last-resort fallback for a filesystem or
// platform that allows neither a symlink nor a hard link to exe.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o755)
}

// pathWithSelfDir returns path with selfDir prepended, so a shell spawned
// inside a Terminal tab can always find `diskette` on its own PATH to run
// the view/sync/tag/untag/select commands (cli.go) — regardless of where
// the running diskette binary happens to live, since it need not be
// installed anywhere in particular. selfDir is left alone if it's already
// on path, so opening several Terminal tabs doesn't pile up duplicates.
func pathWithSelfDir(path, selfDir string) string {
	for _, dir := range filepath.SplitList(path) {
		if dir == selfDir {
			return path
		}
	}
	if path == "" {
		return selfDir
	}
	return selfDir + string(filepath.ListSeparator) + path
}

// shellKindOf identifies which of the shells diskette sync knows how to
// hook (see cli.go's syncSnippet) shellPath is — "" for anything else,
// which cliSync reports as an honest "not supported yet" rather than
// guessing wrong and installing a broken hook.
func shellKindOf(shellPath string) string {
	switch {
	case looksLikeShellPath(shellPath, "zsh"):
		return "zsh"
	case looksLikeShellPath(shellPath, "bash"):
		return "bash"
	default:
		return ""
	}
}

// tabName returns a FileList tab's display name (the directory's own base
// name — a translated "Root" for a filesystem root, which has no
// meaningful base name of its own) or a Terminal tab's, which is always
// just a translated "Terminal": there is no path or title to derive a
// nicer one from without an OSC 0/2 title, which this pane doesn't read.
// Translated once at creation time, like the nav row's own Root button —
// an already-open tab's name doesn't retroactively change if the UI
// language changes later, the one narrow live-retranslation gap this
// program's i18n has (see docs/i18n.md's own "doesn't retroactively
// touch anything already on screen").
func tabName(app *Graphite.Application, kind tabs.Kind, path string) string {
	if kind == tabs.Terminal {
		return app.T(locales.KeyTabTerminal)
	}
	base := filepath.Base(path)
	if base == "." || base == string(filepath.Separator) {
		return app.T(locales.KeyTabRoot)
	}
	return base
}

// paneTabs is one side's whole tab strip and content area: a row of tab
// labels (click to switch, pinned ones marked) at its own top row, and
// whichever tab is active filling the rest — GetChildren returns only
// that one active tab's content, so Window's own focus-cycling and
// hit-test recursion (see graphite's architecture docs) naturally reach
// only it, without paneTabs needing to manually toggle IsFocusable on the
// tabs it isn't showing.
type paneTabs struct {
	Graphite.BaseWidget
	app   *Graphite.Application
	fs    vfs.FileSystem
	group tabs.Group

	// onSearchChanged/onFileListFKey are set once by main() and applied to
	// every FileList tab's FilePane as it's created — a tab created after
	// startup (via the Tab menu) needs the exact same wiring one created
	// at startup already has.
	onSearchChanged func()
	onFileListFKey  func(*filepane.FilePane) func(Graphite.KeyCode)

	// ipcTermRegistry/ipcAddr let AddTerminal register each Terminal tab
	// it spawns (and CloseTabAt unregister it again) so `diskette view`/
	// `sync`/`tag` run inside one can reach back into this instance — see
	// ipc.go. Both are set once, right after construction, once main()
	// has started the IPC server (which itself needs otherGroup, which in
	// turn needs leftTabs/rightTabs to already exist — an ordering this
	// project's other setUp-then-wire fields, like onSearchChanged, share
	// too).
	ipcTermRegistry *terminalRegistry
	ipcAddr         string

	tabRects []tabRect // recomputed every DrawRelative; used by HandleEvent
}

type tabRect struct {
	x0, x1 int
	idx    int
}

// newPaneTabs creates an empty tab group with no tabs yet — main() (or
// restoreSavedTabs) adds the first one right after construction, since a
// pane must always show something.
func newPaneTabs(app *Graphite.Application, fs vfs.FileSystem) *paneTabs {
	p := &paneTabs{BaseWidget: Graphite.NewBaseWidget(0, 0, 0, 0), app: app, fs: fs}
	p.IsFocusable = false
	return p
}

// wireFileList applies this pane's own onSearchChanged/onFileListFKey
// hooks to fp — every FileList tab's FilePane needs them, not just the
// first one created at startup.
func (p *paneTabs) wireFileList(fp *filepane.FilePane) {
	fp.OnSearchChanged = func(string) {
		if p.onSearchChanged != nil {
			p.onSearchChanged()
		}
	}
	fp.OnFunctionKey = func(key Graphite.KeyCode) {
		if p.onFileListFKey != nil {
			p.onFileListFKey(fp)(key)
		}
	}
}

// AddFileList adds a new FileList tab at path and switches to it,
// preserving keyboard focus on this pane if it already had it (matching
// SwitchTo's own behavior, since Add ends by making the new tab active).
func (p *paneTabs) AddFileList(path string) {
	wasFocused := p.HasFocus()
	content := newFileListContent(p.app, p.fs, path)
	p.wireFileList(content.filePane)
	p.group.Add(&tabs.Tab{Kind: tabs.FileList, Name: tabName(p.app, tabs.FileList, path), Widget: content})
	if wasFocused {
		content.focusable().SetFocus(true)
	}
}

// AddTerminal adds a new Terminal tab and switches to it, the same way
// AddFileList does. Reports an error if the shell couldn't be spawned
// (a real OS-level failure — no pty available, the shell binary missing —
// not something to silently swallow).
func (p *paneTabs) AddTerminal() error {
	wasFocused := p.HasFocus()
	content, err := newTerminalContent(p.app, p.ipcAddr)
	if err != nil {
		return err
	}
	if p.ipcTermRegistry != nil {
		p.ipcTermRegistry.register(content.id, p)
	}
	p.group.Add(&tabs.Tab{Kind: tabs.Terminal, Name: tabName(p.app, tabs.Terminal, ""), Widget: content})
	if wasFocused {
		content.focusable().SetFocus(true)
	}
	return nil
}

// AddRemote adds a new Remote tab — a FileList backed by fs (an
// already-connected SFTP or FTP session) instead of this pane's own
// local p.fs — and switches to it, the same way AddFileList/AddTerminal
// do. Deliberately bypasses p.fs entirely rather than reusing
// AddFileList: p.fs is one value shared by every local FileList tab on
// this pane, fixed to vfs.LocalFS, and was never meant to change
// per-tab.
func (p *paneTabs) AddRemote(fs remoteFS, path, label string, meta tabs.SavedRemote) {
	wasFocused := p.HasFocus()
	content := newFileListContent(p.app, fs, path)
	p.wireFileList(content.filePane)
	content.remote = &connectedRemote{label: label, fs: fs, meta: meta}
	p.group.Add(&tabs.Tab{Kind: tabs.Remote, Name: label, Widget: content})
	if wasFocused {
		content.focusable().SetFocus(true)
	}
}

// SwitchTo makes the tab at idx active, moving keyboard focus to its
// content if this pane already had focus (so switching tabs in the pane
// you're already working in doesn't silently kick focus to the other
// pane) and leaving focus alone otherwise (switching a tab in a pane you
// aren't in shouldn't steal it).
func (p *paneTabs) SwitchTo(idx int) {
	wasFocused := p.HasFocus()
	if old, ok := p.contentAt(p.group.Active); ok {
		old.focusable().SetFocus(false)
	}
	p.group.SetActive(idx)
	if wasFocused {
		if newC, ok := p.contentAt(p.group.Active); ok {
			newC.focusable().SetFocus(true)
		}
	}
}

// CloseTabAt closes the tab at idx (refusing to close a pane's last tab,
// same as tabs.Group.Close), preserving focus the same way SwitchTo does
// when closing the active tab moves it to a different one.
func (p *paneTabs) CloseTabAt(idx int) bool {
	wasFocused := p.HasFocus()
	closing, hadContent := p.contentAt(idx)
	if !p.group.Close(idx) {
		return false
	}
	if hadContent && closing.id != "" && p.ipcTermRegistry != nil {
		p.ipcTermRegistry.unregister(closing.id)
	}
	if hadContent && closing.remote != nil {
		closing.remote.fs.Close()
	}
	if wasFocused {
		if c, ok := p.contentAt(p.group.Active); ok {
			c.focusable().SetFocus(true)
		}
	}
	return true
}

// contentAt returns tab idx's tabContent, if idx is in range.
func (p *paneTabs) contentAt(idx int) (tabContent, bool) {
	if idx < 0 || idx >= len(p.group.Tabs) {
		return tabContent{}, false
	}
	c, ok := p.group.Tabs[idx].Widget.(tabContent)
	return c, ok
}

// activeContent returns the active tab's content.
func (p *paneTabs) activeContent() (tabContent, bool) {
	return p.contentAt(p.group.Active)
}

// ActiveFilePane returns the active tab's FilePane, or (nil, false) if
// the active tab is a Terminal instead.
func (p *paneTabs) ActiveFilePane() (*filepane.FilePane, bool) {
	c, ok := p.activeContent()
	if !ok || c.filePane == nil {
		return nil, false
	}
	return c.filePane, true
}

// ActiveTerminal returns the active tab's Terminal, or (nil, false) if
// the active tab is a FileList instead.
func (p *paneTabs) ActiveTerminal() (*Graphite.Terminal, bool) {
	c, ok := p.activeContent()
	if !ok || c.terminal == nil {
		return nil, false
	}
	return c.terminal, true
}

// ActiveRemoteLabel returns the active tab's "user@host" label, or "" if
// it isn't a Remote tab — for diskSpaceBar's third segment.
func (p *paneTabs) ActiveRemoteLabel() string {
	c, ok := p.activeContent()
	if !ok || c.remote == nil {
		return ""
	}
	return c.remote.label
}

// HasFocus overrides BaseWidget.HasFocus: paneTabs itself is never the
// actual focus target (IsFocusable is false), only whichever tab's
// content is currently showing, so "this pane has focus" means "the
// active tab's own focusable widget has focus."
func (p *paneTabs) HasFocus() bool {
	c, ok := p.activeContent()
	if !ok {
		return false
	}
	return c.focusable().HasFocus()
}

// GetChildren implements Widget: only the active tab's content is ever
// reachable by Window's focus-cycling and hit-test recursion — an
// inactive tab isn't drawn, and (per HasFocus's own reasoning above)
// isn't part of the Tab order either.
func (p *paneTabs) GetChildren() []Graphite.Widget {
	c, ok := p.activeContent()
	if !ok {
		return nil
	}
	return []Graphite.Widget{c.display}
}

// tabStripHeight is the fixed one row the tab labels occupy above
// whichever tab's content fills the rest of paneTabs.
const tabStripHeight = 1

// DrawRelative implements Widget.
func (p *paneTabs) DrawRelative(c *Graphite.Canvas, offX, offY, pW, pH int) {
	p.BaseWidget.DrawRelative(c, offX, offY, pW, pH)
	theme := c.Theme()

	for x := 0; x < p.LastW; x++ {
		c.DrawCell(p.AbsX+x, p.AbsY, " ", theme.BgWindow, theme.FgWindow)
	}

	p.tabRects = p.tabRects[:0]
	cursorX := p.AbsX
	for i, t := range p.group.Tabs {
		label := " " + t.Name + " "
		if t.Pinned {
			label = " ●" + t.Name + " " // ● prefix marks a pinned tab
		}
		w := len([]rune(label))
		bg, fg := theme.BgWindow, theme.FgDisabled
		if i == p.group.Active {
			bg, fg = navAccent, navAccent.ContrastText()
		}
		c.DrawTextBounded(cursorX, p.AbsY, w, label, bg, fg)
		p.tabRects = append(p.tabRects, tabRect{x0: cursorX, x1: cursorX + w, idx: i})
		cursorX += w
	}

	if content, ok := p.activeContent(); ok {
		content.display.DrawRelative(c, p.AbsX, p.AbsY+tabStripHeight, p.LastW, p.LastH-tabStripHeight)
	}
}

// DrawOverlay implements Widget: forwarded to the active tab's content
// (a MenuStrip-style dropdown or similar living inside it would otherwise
// never get its overlay pass).
func (p *paneTabs) DrawOverlay(c *Graphite.Canvas, offX, offY, pW, pH int) {
	if content, ok := p.activeContent(); ok {
		content.display.DrawOverlay(c, p.AbsX, p.AbsY+tabStripHeight, p.LastW, p.LastH-tabStripHeight)
	}
}

// HandleEvent implements Widget. Window's own hit-test recursion (see
// GetChildren) already routes a click inside the content area straight to
// the active tab's own content, bypassing this entirely — so by the time
// this runs, ev is always either a click on the tab-strip row itself, or
// something outside both (which HitTest wouldn't have matched in the
// first place, so it can't actually reach here).
func (p *paneTabs) HandleEvent(ev Graphite.Event) {
	if ev.Type != Graphite.EventMouseDown || ev.MouseY != p.AbsY {
		return
	}
	for _, r := range p.tabRects {
		if ev.MouseX >= r.x0 && ev.MouseX < r.x1 {
			p.SwitchTo(r.idx)
			return
		}
	}
}

// newActionGutter builds the fixed-width column between the two panes
// holding Copy, Move, Zip and Unzip — each a dirButton whose label flips
// to always name the focused pane as source and the other one as
// destination, recomputed fresh every frame (there is no "focus changed"
// event to hook, so drawing fresh is what keeps it honest). Zip packs
// src's selection into a .zip landing in dst's directory; Unzip extracts
// an archive selected in src into dst's directory (see doZip/doUnzip and
// internal/archiveengine). A weight-1 spacer above and below the four
// fixed-size buttons centers them in the gutter's full height instead of
// leaving them stacked at the top. Each button's Width 0 stretches it to
// the gutter's full width (see dirButton's own DrawRelative for why that
// requires a custom widget rather than a plain Button).
func newActionGutter(app *Graphite.Application, left, right *paneTabs) *Graphite.Flex {
	// Width 19, not 15: wide enough for "[ Перемістити ▶ ]"/"[ Переместить ▶ ]"/
	// "[ Verplaatsen ▶ ]" (the longest translated gutter label, at 11 runes,
	// across en/uk/ru/nl) without dirButton.DrawRelative's own centering
	// truncating it — 15 only ever fit the English words.
	gutter := Graphite.NewFlex(0, 0, 19, 0, Graphite.FlexColumn)
	gutter.Gap = 1
	gutter.AddChild(Graphite.NewPanel(0, 0, 0, 0), 1)
	gutter.AddChild(newDirButton(app, left, right, locales.KeyGutterCopy, func(src, dst *paneTabs) {
		srcFP, ok1 := src.ActiveFilePane()
		dstFP, ok2 := dst.ActiveFilePane()
		if ok1 && ok2 {
			doCopyOrMove(app, srcFP, dstFP, false)
		}
	}), 0)
	gutter.AddChild(newDirButton(app, left, right, locales.KeyGutterMove, func(src, dst *paneTabs) {
		srcFP, ok1 := src.ActiveFilePane()
		dstFP, ok2 := dst.ActiveFilePane()
		if ok1 && ok2 {
			doCopyOrMove(app, srcFP, dstFP, true)
		}
	}), 0)
	gutter.AddChild(newDirButton(app, left, right, locales.KeyGutterZip, func(src, dst *paneTabs) {
		srcFP, ok1 := src.ActiveFilePane()
		dstFP, ok2 := dst.ActiveFilePane()
		if ok1 && ok2 {
			doZip(app, srcFP, dstFP)
		}
	}), 0)
	gutter.AddChild(newDirButton(app, left, right, locales.KeyGutterUnzip, func(src, dst *paneTabs) {
		srcFP, ok1 := src.ActiveFilePane()
		dstFP, ok2 := dst.ActiveFilePane()
		if ok1 && ok2 {
			doUnzip(app, srcFP, dstFP)
		}
	}), 0)
	gutter.AddChild(Graphite.NewPanel(0, 0, 0, 0), 1)
	return gutter
}

// dirButton is a gutter button whose label names whichever pane currently
// has focus as the source and the other one as the destination, recomputed
// every frame. All four gutter buttons (Copy, Move, Zip, Unzip) wire a
// real onClick; a nil one renders dimmed and inert instead, a fallback
// this project doesn't currently exercise but keeps available for a
// future action that isn't always applicable. Deliberately not a
// Graphite.Button: a Button's background only fills the exact width its
// own text occupies, not whatever extra width a Flex weight hands it, and
// its Text is a plain field with no per-frame hook for a label that must
// track live state.
type dirButton struct {
	Graphite.BaseWidget
	app         *Graphite.Application
	left, right *paneTabs
	actionKey   string
	onClick     func(src, dst *paneTabs)
}

// newDirButton creates a dirButton with Width 0, stretching it to fill
// whatever width its parent Flex offers (see BaseWidget's zero/negative
// width convention). A nil onClick renders it dimmed and inert. actionKey
// is a locales.Key* constant, resolved fresh in label() every frame —
// the same live-retranslation-for-free pattern FilePane.Translate and
// the F-key bar's OnBeforeDraw both use, since this label is already
// recomputed every frame for the source/destination arrow anyway.
func newDirButton(app *Graphite.Application, left, right *paneTabs, actionKey string, onClick func(src, dst *paneTabs)) *dirButton {
	base := Graphite.NewBaseWidget(0, 0, 0, 1)
	return &dirButton{BaseWidget: base, app: app, left: left, right: right, actionKey: actionKey, onClick: onClick}
}

// srcDst returns (source, destination) for this click: always from
// whichever pane is focused toward the other one.
func (d *dirButton) srcDst() (src, dst *paneTabs) {
	if d.right.HasFocus() {
		return d.right, d.left
	}
	return d.left, d.right
}

// label returns this frame's button text: the arrow sits on whichever side
// faces the destination pane — trailing ("Copy ▶") when copying rightward,
// leading ("◀ Copy") when copying leftward — so the glyph itself points
// toward where the files are actually going.
func (d *dirButton) label() string {
	action := d.app.T(d.actionKey)
	if d.right.HasFocus() {
		return "◀ " + action
	}
	return action + " ▶"
}

// DrawRelative implements Graphite.Widget. The label is centered in the
// button's full (stretched) width rather than left-aligned, so it reads as
// centered content inside a wide button instead of hugging one edge.
func (d *dirButton) DrawRelative(c *Graphite.Canvas, offX, offY, pW, pH int) {
	d.BaseWidget.DrawRelative(c, offX, offY, pW, pH)
	theme := c.Theme()
	bg, fg := navAccent, navAccent.ContrastText()
	if d.onClick == nil {
		bg, fg = theme.Disabled, theme.FgDisabled
	}
	for x := 0; x < d.LastW; x++ {
		c.DrawCell(d.AbsX+x, d.AbsY, " ", bg, fg)
	}
	text := "[ " + d.label() + " ]"
	pad := (d.LastW - len([]rune(text))) / 2
	if pad < 0 {
		pad = 0
	}
	c.DrawTextBounded(d.AbsX+pad, d.AbsY, d.LastW-pad, text, bg, fg)
}

// HandleEvent implements Graphite.Widget: a click runs onClick with
// srcDst's direction as of this exact click, not whatever it was when the
// button was constructed. A nil onClick makes the button inert.
func (d *dirButton) HandleEvent(ev Graphite.Event) {
	if ev.Type != Graphite.EventMouseDown || d.onClick == nil {
		return
	}
	src, dst := d.srcDst()
	d.onClick(src, dst)
}

// diskUsageCache is one disk-usage query's result — recomputed fresh
// every frame by diskSpaceBar's own state callback rather than cached, now
// that "which pane/tab is active" can change from more than just
// navigation (switching tabs doesn't fire FilePane.OnPathChanged, so a
// cache keyed off that would go stale the moment tabs entered the
// picture) — a local statfs is cheap enough that recomputing it ~100
// times a second is not a real cost.
type diskUsageCache struct {
	usage diskspace.Usage
	ok    bool
}

// statusBarState is one frame's worth of everything the bottom status row
// needs: the tagged-selection summary for whichever pane is focused, and
// that pane's disk usage, and remoteLabel (e.g. "me@example.com") for the
// third segment when the focused pane's active tab is a Remote FileList —
// empty means "Local". usage/taggedCount/taggedSize/hasTagged are all the
// zero value when the active tab is a Terminal (nothing file-related to
// report) or a Remote FileList (disk usage is a local-filesystem-only
// query — internal/diskspace.Query — that doesn't mean anything for an
// SFTP path; only tagging still applies there).
type statusBarState struct {
	usage       diskUsageCache
	taggedCount int
	taggedSize  int64
	hasTagged   bool
	remoteLabel string
}

// diskSpaceBar is the full-width row above the F-key bar, split into three
// parts separated by "│": tagged-selection size, a disk usage progress
// bar, and a third segment showing "Local" or, for a Remote tab, its
// "user@host" label (see statusBarState.remoteLabel). state is read fresh
// every frame (see dirButton for why: there is no "focus changed" hook to
// update from instead).
type diskSpaceBar struct {
	Graphite.BaseWidget
	app   *Graphite.Application
	state func() statusBarState
}

// newDiskSpaceBar creates the bar at Y=-3: one row above the very bottom
// (the F-key bar itself, at Y=-1), leaving row -2 blank — the same
// one-row gap the menu strip and nav row already have between them —
// instead of the two rows touching directly.
func newDiskSpaceBar(app *Graphite.Application, state func() statusBarState) *diskSpaceBar {
	return &diskSpaceBar{BaseWidget: Graphite.NewBaseWidget(0, -3, 0, 1), app: app, state: state}
}

// formatBytes renders a byte count in the largest binary unit (KiB, MiB,
// ...) that keeps the number under 1024, matching how disk sizes are
// conventionally shown.
func formatBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// DrawRelative implements Graphite.Widget.
func (d *diskSpaceBar) DrawRelative(c *Graphite.Canvas, offX, offY, pW, pH int) {
	d.BaseWidget.DrawRelative(c, offX, offY, pW, pH)
	theme := c.Theme()
	for x := 0; x < d.LastW; x++ {
		c.DrawCell(d.AbsX+x, d.AbsY, " ", theme.BgWindow, theme.FgWindow)
	}

	const (
		taggedSegW = 28
		serverSegW = 20
	)
	diskSegW := d.LastW - taggedSegW - serverSegW - 2 // 2 dividers, 1 column each
	if diskSegW < 20 {
		// Too narrow for three segments to mean anything — fall back to
		// just the disk segment, full width, rather than three
		// unreadably squeezed fragments.
		d.drawDiskSegment(c, d.AbsX, d.LastW)
		return
	}

	state := d.state()

	taggedText := d.app.T(locales.KeyStatusNoTagged)
	if state.hasTagged {
		taggedText = d.app.T(locales.KeyStatusTagged, state.taggedCount, formatBytes(uint64(state.taggedSize)))
	}
	c.DrawTextBounded(d.AbsX, d.AbsY, taggedSegW, taggedText, theme.BgWindow, theme.FgWindow)

	dividerX1 := d.AbsX + taggedSegW
	c.DrawCell(dividerX1, d.AbsY, "│", theme.BgWindow, theme.FgDisabled)

	diskX := dividerX1 + 1
	d.drawDiskSegmentUsage(c, diskX, diskSegW, state.usage)

	dividerX2 := diskX + diskSegW
	c.DrawCell(dividerX2, d.AbsY, "│", theme.BgWindow, theme.FgDisabled)

	serverText := d.app.T(locales.KeyStatusServer, d.app.T(locales.KeyStatusServerLocal))
	if state.remoteLabel != "" {
		serverText = d.app.T(locales.KeyStatusServer, state.remoteLabel)
	}
	c.DrawTextBounded(dividerX2+1, d.AbsY, serverSegW-1, serverText, theme.BgWindow, theme.FgDisabled)
}

// drawDiskSegment resolves state itself, for the narrow-terminal fallback
// path that skips the tagged/server segments entirely.
func (d *diskSpaceBar) drawDiskSegment(c *Graphite.Canvas, x, w int) {
	d.drawDiskSegmentUsage(c, x, w, d.state().usage)
}

// drawDiskSegmentUsage draws the "Disk: free of total (N% used) [bar]"
// segment at x, within width w.
func (d *diskSpaceBar) drawDiskSegmentUsage(c *Graphite.Canvas, x, w int, cache diskUsageCache) {
	theme := c.Theme()
	if !cache.ok || cache.usage.Total == 0 {
		c.DrawTextBounded(x, d.AbsY, w, d.app.T(locales.KeyStatusDiskNA), theme.BgWindow, theme.FgDisabled)
		return
	}

	used := cache.usage.Total - cache.usage.Free
	usedPct := float64(used) / float64(cache.usage.Total) * 100
	label := d.app.T(locales.KeyStatusDiskUsage, formatBytes(cache.usage.Free), formatBytes(cache.usage.Total), usedPct)

	barW := w - len([]rune(label)) - 3
	if barW < 10 {
		c.DrawTextBounded(x, d.AbsY, w, label, theme.BgWindow, theme.FgWindow)
		return
	}

	c.DrawTextBounded(x, d.AbsY, len([]rune(label)), label, theme.BgWindow, theme.FgWindow)
	barX := x + len([]rune(label)) + 1
	c.DrawCell(barX, d.AbsY, "[", theme.BgWindow, theme.FgWindow)
	barX++
	filled := int(usedPct / 100 * float64(barW))
	barColor := theme.Primary
	if usedPct >= 90 {
		barColor = theme.Danger
	} else if usedPct >= 75 {
		barColor = theme.Warning
	}
	for i := 0; i < barW; i++ {
		if i < filled {
			c.DrawCell(barX+i, d.AbsY, "█", theme.BgWindow, barColor)
		} else {
			c.DrawCell(barX+i, d.AbsY, "░", theme.BgWindow, theme.Disabled)
		}
	}
	c.DrawCell(barX+barW, d.AbsY, "]", theme.BgWindow, theme.FgWindow)
}

// newFKeyBar builds the bottom action bar. F9 is listed with no OnClick
// (fkeybar renders it dimmed and inert) since Menu wasn't required by an
// earlier phase's scope — showing it dimmed is honest about that instead
// of quietly leaving it off the bar's layout. Every active key is
// RolePrimary (the palette's lime accent) except Delete, which is
// destructive and stays RoleDanger.
// withFP/withPanes are the same active-FileList guards main() builds for
// the menu, reused here so F1/F2/F3/F7/F8 (and F5/F6's dst lookup) all
// share one no-op-on-a-Terminal-tab behavior.
func newFKeyBar(app *Graphite.Application, right *paneTabs, withFP func(func(*filepane.FilePane)) func(), withPanes func(func(src, dst *filepane.FilePane)) func()) *fkeybar.Bar {
	bar := fkeybar.New(0, -1, []fkeybar.Key{
		{Label: "F1", OnClick: withFP(func(fp *filepane.FilePane) { showFileInfo(app, fp) })},
		{Label: "F2", OnClick: withFP(func(fp *filepane.FilePane) { doRename(app, fp) })},
		{Label: "F3", OnClick: withFP(func(fp *filepane.FilePane) { showFindFiles(app, fp) })},
		{Label: "F4", OnClick: withFP(func(fp *filepane.FilePane) { showGrepSearch(app, fp) })},
		{Label: "F5", OnClick: withPanes(func(src, dst *filepane.FilePane) { doCopyOrMove(app, src, dst, false) })},
		{Label: "F6", OnClick: withPanes(func(src, dst *filepane.FilePane) { doCopyOrMove(app, src, dst, true) })},
		{Label: "F7", OnClick: withFP(func(fp *filepane.FilePane) { doMkdir(app, fp) })},
		{Label: "F8", Role: fkeybar.RoleDanger, OnClick: withFP(func(fp *filepane.FilePane) { doDelete(app, fp) })},
		{Label: "F9"},
		{Label: "F10"},
	})
	// Every key's Text (not just F5/F6's direction-qualified one) is
	// recomputed from app.T on every single draw — the same per-frame
	// pattern F5/F6's own "which pane is the destination" text already
	// needed, extended to the whole bar so a locale change (see the Help
	// menu's Language submenu in newMenuStrip) is reflected the very next
	// frame with no separate "retranslate the F-key bar" step.
	const copyIdx, moveIdx = 4, 5
	bar.OnBeforeDraw = func() {
		bar.Keys[0].Text = app.T(locales.KeyFKeyInfo)
		bar.Keys[1].Text = app.T(locales.KeyFKeyRename)
		bar.Keys[2].Text = app.T(locales.KeyFKeyFind)
		bar.Keys[3].Text = app.T(locales.KeyFKeyGrep)
		dir := app.T(locales.KeyDirectionRight)
		if right.HasFocus() {
			dir = app.T(locales.KeyDirectionLeft)
		}
		bar.Keys[copyIdx].Text = app.T(locales.KeyFKeyCopy, dir)
		bar.Keys[moveIdx].Text = app.T(locales.KeyFKeyMove, dir)
		bar.Keys[6].Text = app.T(locales.KeyFKeyMkdir)
		bar.Keys[7].Text = app.T(locales.KeyFKeyDelete)
		bar.Keys[8].Text = app.T(locales.KeyFKeyMenu)
		bar.Keys[9].Text = app.T(locales.KeyFKeyQuit)
	}
	return bar
}

// newMenuStrip builds the top menu bar as a mouse-only duplicate of the
// F-key actions and a few operations that otherwise only have a keyboard
// or header-click path, plus the new Tab category (Add's New file
// list/New terminal submenu, Manage Tabs, Pin/Unpin, Close) — the reason
// MenuStrip grew separator/SubItems support in the first place.
// IsFocusable is forced back to false right after construction, same
// reasoning as newNavButton.
func newMenuStrip(app *Graphite.Application, left, right *paneTabs, active func() *paneTabs, withFP func(func(*filepane.FilePane)) func(), withPanes func(func(src, dst *filepane.FilePane)) func()) *Graphite.MenuStrip {
	// menu.Categories is rebuilt in place (see retranslateMenu) every time
	// the Language submenu below changes app's Locale — a MenuItem.Label
	// is otherwise a plain string fixed at construction time, unlike the
	// F-key bar's Text (recomputed every frame via OnBeforeDraw) or a
	// FilePane's headers (recomputed every frame via Translate), neither
	// of which needs this. buildCategories/switchLocale/retranslateMenu
	// are declared as vars up front, in that forward-reference order,
	// purely so each closure below can already name the next one it
	// calls — every var is assigned its real function before app.Run()
	// ever lets one of them actually execute.
	var menu *Graphite.MenuStrip
	var buildCategories func() []Graphite.MenuCategory
	var switchLocale func(Graphite.Locale)
	var retranslateMenu func()

	buildCategories = func() []Graphite.MenuCategory {
		return []Graphite.MenuCategory{
			{Label: app.T(locales.KeyMenuFile), Items: []Graphite.MenuItem{
				{Label: app.T(locales.KeyMenuFileInfo), Action: withFP(func(fp *filepane.FilePane) { showFileInfo(app, fp) })},
				{Label: app.T(locales.KeyMenuFileRename), Action: withFP(func(fp *filepane.FilePane) { doRename(app, fp) })},
				{Label: app.T(locales.KeyMenuFileFind), Action: withFP(func(fp *filepane.FilePane) { showFindFiles(app, fp) })},
				{Label: app.T(locales.KeyMenuFileGrep), Action: withFP(func(fp *filepane.FilePane) { showGrepSearch(app, fp) })},
				{Separator: true},
				{Label: app.T(locales.KeyMenuFileCopy), Action: withPanes(func(src, dst *filepane.FilePane) { doCopyOrMove(app, src, dst, false) })},
				{Label: app.T(locales.KeyMenuFileMove), Action: withPanes(func(src, dst *filepane.FilePane) { doCopyOrMove(app, src, dst, true) })},
				{Separator: true},
				{Label: app.T(locales.KeyMenuFileNew), Action: withFP(func(fp *filepane.FilePane) { doNewFile(app, fp) })},
				{Label: app.T(locales.KeyMenuFileMkdir), Action: withFP(func(fp *filepane.FilePane) { doMkdir(app, fp) })},
				{Label: app.T(locales.KeyMenuFileDelete), Action: withFP(func(fp *filepane.FilePane) { doDelete(app, fp) })},
				{Separator: true},
				{Label: app.T(locales.KeyMenuFileQuit), Action: func() { requestQuit(app, left, right) }},
			}},
			{Label: app.T(locales.KeyMenuMark), Items: []Graphite.MenuItem{
				{Label: app.T(locales.KeyMenuMarkToggle), Action: withFP(func(fp *filepane.FilePane) { fp.ToggleTag() })},
				{Separator: true},
				{Label: app.T(locales.KeyMenuMarkAll), Action: withFP(func(fp *filepane.FilePane) { fp.SelectAll() })},
				{Label: app.T(locales.KeyMenuMarkNone), Action: withFP(func(fp *filepane.FilePane) { fp.DeselectAll() })},
				{Label: app.T(locales.KeyMenuMarkInvert), Action: withFP(func(fp *filepane.FilePane) { fp.InvertSelection() })},
			}},
			{Label: app.T(locales.KeyMenuView), Items: []Graphite.MenuItem{
				{Label: app.T(locales.KeyMenuViewSortName), Action: withFP(func(fp *filepane.FilePane) { fp.SetSort(filepane.SortByName) })},
				{Label: app.T(locales.KeyMenuViewSortExt), Action: withFP(func(fp *filepane.FilePane) { fp.SetSort(filepane.SortByExt) })},
				{Label: app.T(locales.KeyMenuViewSortSize), Action: withFP(func(fp *filepane.FilePane) { fp.SetSort(filepane.SortBySize) })},
				{Label: app.T(locales.KeyMenuViewSortDate), Action: withFP(func(fp *filepane.FilePane) { fp.SetSort(filepane.SortByDate) })},
				{Separator: true},
				{Label: app.T(locales.KeyMenuViewRefresh), Action: withFP(func(fp *filepane.FilePane) { fp.Reload() })},
			}},
			{Label: app.T(locales.KeyMenuTab), Items: []Graphite.MenuItem{
				{Label: app.T(locales.KeyMenuTabAdd), SubItems: []Graphite.MenuItem{
					{Label: app.T(locales.KeyMenuTabAddFileList), Action: func() {
						if fp, ok := active().ActiveFilePane(); ok {
							active().AddFileList(fp.Path())
						} else {
							active().AddFileList(mustGetwd())
						}
					}},
					{Label: app.T(locales.KeyMenuTabAddTerminal), Action: func() {
						if err := active().AddTerminal(); err != nil {
							app.ShowMessage(app.T(locales.KeyErrorTitle), err.Error(), Graphite.BtnDanger)
						}
					}},
				}},
				{Separator: true},
				{Label: app.T(locales.KeyMenuTabPinUnpin), Action: func() { active().group.TogglePin(active().group.Active) }},
				{Label: app.T(locales.KeyMenuTabClose), Action: func() { active().CloseTabAt(active().group.Active) }},
				{Separator: true},
				{Label: app.T(locales.KeyMenuTabManage), Action: func() { showManageTabs(app, left, right) }},
			}},
			{Label: app.T(locales.KeyMenuNetwork), Items: []Graphite.MenuItem{
				{Label: app.T(locales.KeyMenuNetworkCreate), Action: func() {
					showConnectDialog(app, active(), nil, "/", nil)
				}},
				{Label: app.T(locales.KeyMenuNetworkReconnect), Action: func() { doReconnect(app, active()) }},
				{Label: app.T(locales.KeyMenuNetworkDisconnect), Action: func() { doDisconnect(app, active()) }},
			}},
			{Label: app.T(locales.KeyMenuHelp), Items: []Graphite.MenuItem{
				{Label: app.T(locales.KeyMenuHelpLanguage), SubItems: []Graphite.MenuItem{
					// Each language's own name is never translated — a
					// picker always lists its options in their own
					// language (an English speaker still recognizes
					// "Українська" as a name, not as English text to
					// read), the same convention every OS/app language
					// switcher follows.
					{Label: "English", Action: func() { switchLocale(Graphite.LocaleEnglish) }},
					{Label: "Українська", Action: func() { switchLocale(Graphite.LocaleUkrainian) }},
					{Label: "Русский", Action: func() { switchLocale(Graphite.LocaleRussian) }},
					{Label: "Nederlands", Action: func() { switchLocale(Graphite.LocaleDutch) }},
				}},
				{Separator: true},
				{Label: app.T(locales.KeyMenuHelpAbout), Action: func() { showAbout(app) }},
			}},
		}
	}

	switchLocale = func(loc Graphite.Locale) {
		app.SetLocale(loc)
		saveTabs(left, right) // persists immediately, not just at quit — see installSignalSaveHandler's own reasoning
		retranslateMenu()
	}

	retranslateMenu = func() {
		menu.Categories = buildCategories()
	}

	menu = Graphite.NewMenuStrip(buildCategories())
	menu.IsFocusable = false
	menu.BgColor = Graphite.Hex("#FFD23D") // amber, per the project owner — bar and dropdown alike; text auto-contrasts
	return menu
}

// mustGetwd is Add's fallback root for a brand new FileList tab opened
// from a pane whose active tab is a Terminal (so there's no "current
// path" to inherit) — the process's own working directory, the same
// starting point the very first tab of each pane uses.
func mustGetwd() string {
	dir, err := os.Getwd()
	if err != nil {
		return string(filepath.Separator)
	}
	return dir
}

// requestQuit asks the user to confirm before actually quitting — wired to
// both Escape (via Application.SetOnQuitRequested) and F10. Saves pinned
// tabs first (see saveTabs), so "protected from reset on restart" holds
// even for a quit the user didn't cancel out of.
func requestQuit(app *Graphite.Application, left, right *paneTabs) {
	Graphite.ShowConfirm(app, app.T(locales.KeyQuitTitle), app.T(locales.KeyQuitMessage), Graphite.BtnDanger, func() {
		saveAndQuit(app, left, right)
	})
}

// saveAndQuit does everything a shutdown needs regardless of how it was
// triggered — the confirmed Quit dialog, or runTUI's own SIGINT/SIGTERM
// handler for a terminal window closing or a plain Ctrl+C, neither of
// which goes through requestQuit's confirmation at all. Saving pinned
// tabs only on the confirmed path (as this used to) meant closing the
// terminal window instead of using F10/Escape silently skipped it —
// tabs.Save never ran, so nothing was there to restore next launch
// regardless of what was pinned.
func saveAndQuit(app *Graphite.Application, left, right *paneTabs) {
	saveTabs(left, right)
	closeRemoteTabs(left)
	closeRemoteTabs(right)
	if cliShim.dir != "" {
		os.RemoveAll(cliShim.dir)
	}
	if zshSyncShim.dir != "" {
		os.RemoveAll(zshSyncShim.dir)
	}
	if bashSyncShim.path != "" {
		os.RemoveAll(filepath.Dir(bashSyncShim.path))
	}
	app.Quit()
}

// closeRemoteTabs closes the SFTP session behind every Remote tab still
// open on p, called before quitting — mirrors CloseTabAt's own cleanup
// for a tab closed one at a time, since quitting never goes through
// CloseTabAt for the tabs still open when it happens.
func closeRemoteTabs(p *paneTabs) {
	for _, t := range p.group.Tabs {
		if c, ok := t.Widget.(tabContent); ok && c.remote != nil {
			c.remote.fs.Close()
		}
	}
}

// showAbout opens a custom modal: the logo (internal/assets.DisketteLogo,
// embedded into the binary at compile time, decoded once here) on the
// left, program information on the right. No version number is shown —
// AGENTS_UNIVERSAL reserves the version-bump decision for the project
// owner, and none has been authorized yet; a "development build" label is
// truthful without inventing one.
func showAbout(app *Graphite.Application) {
	// Height 27: the logo is 18 rows starting at content row 2 (through
	// row 19), so the content area needs to be at least that tall plus
	// room for the Close button below it, not just enough for the info
	// text — a shorter window here left the button drawn on top of the
	// image's own bottom rows.
	mod := Graphite.NewWindow(94, 27, " "+app.T(locales.KeyMenuHelpAbout)+" ")

	if logo, err := Graphite.ReadGph(bytes.NewReader(assets.DisketteLogo)); err == nil {
		mod.AddWidget(Graphite.NewImage(2, 2, logo))
	}

	info := Graphite.NewLabel(40, 2,
		app.T(locales.KeyAboutVersion, "0.2.0")+"\n\n"+
			app.T(locales.KeyAboutTagline1)+"\n"+
			app.T(locales.KeyAboutTagline2)+"\n"+
			app.T(locales.KeyAboutTagline3)+"\n"+
			app.T(locales.KeyAboutTagline4)+"\n\n"+
			"Copyright © 2026 Yehor Oblyvantsov\n"+
			"github.com/yeoblyv/diskette\n\n"+
			app.T(locales.KeyAboutBeta))
	info.Width = 44
	mod.AddWidget(info)

	mod.AddWidget(Graphite.NewButton(2, -2, app.T(locales.KeyClose), Graphite.BtnDefault, func() {
		app.CloseModal()
	}))

	app.SetModal(mod)
}

// showFileInfo implements F1: full metadata (size, permissions, modified/
// accessed/created) for a single selected entry, or just a count and
// total size when more than one is tagged — the same "detail vs. summary"
// split TaggedSummary/SelectionPaths already establish for F5/F6/F8.
func showFileInfo(app *Graphite.Application, fp *filepane.FilePane) {
	paths := fp.SelectionPaths()
	if len(paths) == 0 {
		return // the cursor is on ".." — nothing to report on
	}

	if len(paths) > 1 {
		count, size, _ := fp.TaggedSummary()
		mod := Graphite.NewWindow(44, 10, app.T(locales.KeyFileInfoTitle))
		mod.AddWidget(Graphite.NewLabel(2, 1, app.T(locales.KeyFileInfoMulti, count, formatBytes(uint64(size)))))
		mod.AddWidget(Graphite.NewButton(2, -2, app.T(locales.KeyClose), Graphite.BtnDefault, func() {
			app.CloseModal()
		}))
		app.SetModal(mod)
		return
	}

	info, err := fileinfo.Stat(paths[0])
	if err != nil {
		app.ShowMessage(app.T(locales.KeyErrorTitle), err.Error(), Graphite.BtnDanger)
		return
	}

	kind := app.T(locales.KeyFileInfoKindFile)
	sizeStr := formatBytes(uint64(info.Size))
	if info.IsDir {
		kind, sizeStr = app.T(locales.KeyFileInfoKindDir), "—"
	}
	created := app.T(locales.KeyFileInfoNotAvail)
	if info.CreatedKnown {
		created = info.Created.Format("02.01.2006 15:04:05")
	}

	const layout = "02.01.2006 15:04:05"
	text := strings.Join([]string{
		app.T(locales.KeyFileInfoName, info.Name),
		app.T(locales.KeyFileInfoType, kind),
		app.T(locales.KeyFileInfoSize, sizeStr),
		app.T(locales.KeyFileInfoPerms, info.Mode.String()),
		"",
		app.T(locales.KeyFileInfoModified, info.Modified.Format(layout)),
		app.T(locales.KeyFileInfoAccessed, info.Accessed.Format(layout)),
		app.T(locales.KeyFileInfoCreated, created),
	}, "\n")

	mod := Graphite.NewWindow(56, 15, app.T(locales.KeyFileInfoTitle))
	mod.AddWidget(Graphite.NewLabel(2, 1, text))
	mod.AddWidget(Graphite.NewButton(2, -2, app.T(locales.KeyClose), Graphite.BtnDefault, func() {
		app.CloseModal()
	}))
	app.SetModal(mod)
}

// showFindFiles implements F3: a criteria modal (search root, name mask,
// recurse/case-sensitive toggles) that hands off to runFileSearch once
// submitted.
func showFindFiles(app *Graphite.Application, fp *filepane.FilePane) {
	mod := Graphite.NewWindow(60, 14, app.T(locales.KeyFindTitle))

	rootInput := Graphite.NewInputBox(2, 1, 54, app.T(locales.KeySearchIn))
	rootInput.Value = fp.Path()
	mod.AddWidget(rootInput)

	maskInput := Graphite.NewInputBox(2, 3, 54, app.T(locales.KeyNameMask))
	maskInput.Value = "*"
	mod.AddWidget(maskInput)

	recurseBox := Graphite.NewCheckbox(2, 5, app.T(locales.KeySearchSubfolders), true)
	mod.AddWidget(recurseBox)
	caseBox := Graphite.NewCheckbox(2, 6, app.T(locales.KeyCaseSensitive), false)
	mod.AddWidget(caseBox)

	mod.AddWidget(Graphite.NewButton(2, -2, app.T(locales.KeySearch), Graphite.BtnSuccess, func() {
		app.CloseModal()
		runFileSearch(app, fp, search.Options{
			Mask:          maskInput.Value,
			Recursive:     recurseBox.Checked,
			CaseSensitive: caseBox.Checked,
		}, rootInput.Value)
	}))
	mod.AddWidget(Graphite.NewButton(14, -2, app.T(locales.KeyCancel), Graphite.BtnDefault, func() {
		app.CloseModal()
	}))

	app.SetModal(mod)
}

// runFileSearch runs opts against root in the background (so a large tree
// doesn't freeze the UI) and shows results as they arrive in a live-
// updating list; selecting one navigates fp there and hands the match to
// FilePane.SetFound, so it reads as a search result rather than an
// ordinary cursor move.
func runFileSearch(app *Graphite.Application, fp *filepane.FilePane, opts search.Options, root string) {
	mod := Graphite.NewWindow(64, 20, app.T(locales.KeyFindResultsTitle))
	status := Graphite.NewLabel(2, 1, app.T(locales.KeySearching))
	mod.AddWidget(status)

	results := Graphite.NewListBox(2, 3, -4, -4, nil, func(_ int, path string) {
		app.CloseModal()
		dir, ok := fp.FS.Parent(path)
		if !ok {
			return
		}
		name := path[len(dir):]
		name = trimLeadingSeparators(name)
		fp.SetPath(dir)
		fp.SetFound(name)
	})
	mod.AddWidget(results)

	ctx, cancel := context.WithCancel(context.Background())
	mod.AddWidget(Graphite.NewButton(2, -2, app.T(locales.KeyCancel), Graphite.BtnDefault, func() {
		cancel()
		app.CloseModal()
	}))
	app.SetModal(mod)

	go func() {
		var matches []string
		lastUpdate := time.Now()
		flush := func(done bool) {
			snapshot := append([]string(nil), matches...)
			app.Invoke(func() {
				results.Items = snapshot
				if done {
					status.SetText(app.T(locales.KeySearchFound, len(snapshot)))
				} else {
					status.SetText(app.T(locales.KeySearchingFound, len(snapshot)))
				}
			})
		}

		err := search.Run(ctx, fp.FS, root, opts, func(m search.Match) {
			matches = append(matches, m.Path)
			if time.Since(lastUpdate) > 150*time.Millisecond {
				flush(false)
				lastUpdate = time.Now()
			}
		})
		flush(true)
		if err != nil && ctx.Err() == nil {
			app.Invoke(func() { app.ShowMessage(app.T(locales.KeyErrorTitle), err.Error(), Graphite.BtnDanger) })
		}
	}()
}

// showGrepSearch implements F4: a grep-style search of file contents,
// prompting for a pattern (literal or regular expression), an optional
// name mask to restrict which files are searched, and the same
// recurse/case-sensitive choices showFindFiles offers.
func showGrepSearch(app *Graphite.Application, fp *filepane.FilePane) {
	mod := Graphite.NewWindow(60, 16, app.T(locales.KeyGrepTitle))

	rootInput := Graphite.NewInputBox(2, 1, 54, app.T(locales.KeySearchIn))
	rootInput.Value = fp.Path()
	mod.AddWidget(rootInput)

	patternInput := Graphite.NewInputBox(2, 3, 54, app.T(locales.KeyGrepPattern))
	mod.AddWidget(patternInput)

	maskInput := Graphite.NewInputBox(2, 5, 54, app.T(locales.KeyNameMask))
	maskInput.Value = "*"
	mod.AddWidget(maskInput)

	recurseBox := Graphite.NewCheckbox(2, 7, app.T(locales.KeySearchSubfolders), true)
	mod.AddWidget(recurseBox)
	caseBox := Graphite.NewCheckbox(2, 8, app.T(locales.KeyCaseSensitive), false)
	mod.AddWidget(caseBox)
	regexBox := Graphite.NewCheckbox(2, 9, app.T(locales.KeyGrepRegex), false)
	mod.AddWidget(regexBox)

	mod.AddWidget(Graphite.NewButton(2, -2, app.T(locales.KeySearch), Graphite.BtnSuccess, func() {
		if patternInput.Value == "" {
			return
		}
		app.CloseModal()
		runGrepSearch(app, fp, grep.Options{
			Pattern:       patternInput.Value,
			Regex:         regexBox.Checked,
			CaseSensitive: caseBox.Checked,
			Mask:          maskInput.Value,
			Recursive:     recurseBox.Checked,
		}, rootInput.Value)
	}))
	mod.AddWidget(Graphite.NewButton(14, -2, app.T(locales.KeyCancel), Graphite.BtnDefault, func() {
		app.CloseModal()
	}))

	app.SetModal(mod)
}

// runGrepSearch runs opts against root in the background (so a large
// tree doesn't freeze the UI) and shows results as they arrive in a
// live-updating list, one per matching line; selecting one navigates fp
// to that file the same way runFileSearch's results do.
func runGrepSearch(app *Graphite.Application, fp *filepane.FilePane, opts grep.Options, root string) {
	mod := Graphite.NewWindow(78, 20, app.T(locales.KeyGrepResultsTitle))
	status := Graphite.NewLabel(2, 1, app.T(locales.KeySearching))
	mod.AddWidget(status)

	// shown is only ever written from within app.Invoke and read from
	// results' onSelect — both run on the UI goroutine — so it never
	// touches the background goroutine below, which keeps its own
	// unshared allMatches/allLines instead. That split avoids a data race
	// that a single slice written by both goroutines would otherwise have.
	var shown []grep.Match
	results := Graphite.NewListBox(2, 3, -4, -4, nil, func(idx int, _ string) {
		if idx < 0 || idx >= len(shown) {
			return
		}
		m := shown[idx]
		app.CloseModal()
		dir, ok := fp.FS.Parent(m.Path)
		if !ok {
			return
		}
		name := trimLeadingSeparators(m.Path[len(dir):])
		fp.SetPath(dir)
		fp.SetFound(name)
	})
	mod.AddWidget(results)

	ctx, cancel := context.WithCancel(context.Background())
	mod.AddWidget(Graphite.NewButton(2, -2, app.T(locales.KeyCancel), Graphite.BtnDefault, func() {
		cancel()
		app.CloseModal()
	}))
	app.SetModal(mod)

	go func() {
		var allMatches []grep.Match
		var allLines []string
		lastUpdate := time.Now()
		flush := func(done bool) {
			snapshotMatches := append([]grep.Match(nil), allMatches...)
			snapshotLines := append([]string(nil), allLines...)
			app.Invoke(func() {
				shown = snapshotMatches
				results.Items = snapshotLines
				if done {
					status.SetText(app.T(locales.KeySearchFound, len(snapshotLines)))
				} else {
					status.SetText(app.T(locales.KeySearchingFound, len(snapshotLines)))
				}
			})
		}

		err := grep.Run(ctx, fp.FS, root, opts, func(m grep.Match) {
			allMatches = append(allMatches, m)
			allLines = append(allLines, fmt.Sprintf("%s:%d: %s", m.Path, m.LineNum, strings.TrimSpace(m.Line)))
			if time.Since(lastUpdate) > 150*time.Millisecond {
				flush(false)
				lastUpdate = time.Now()
			}
		})
		flush(true)
		if err != nil && ctx.Err() == nil {
			app.Invoke(func() { app.ShowMessage(app.T(locales.KeyErrorTitle), err.Error(), Graphite.BtnDanger) })
		}
	}()
}

// nextButtonX returns where the next button in a horizontal row should
// start, given the previous one's own X and its label: wide enough for
// that button's own rendered "[ label ]" plus a 2-column gap, so two
// buttons never collide regardless of how much longer a translation runs
// than the English word a hand-picked offset (e.g. the old fixed 14, 21,
// 30 in a Yes/No/Rename/Cancel row) was originally tuned for — Ukrainian
// and Russian in particular run noticeably longer than English for verbs
// like "Rename"/"Overwrite". Graphite's Button itself renders exactly
// "[ " + label + " ]", so this mirrors that layout rather than
// introducing a second convention.
func nextButtonX(prevX int, prevLabel string) int {
	const brackets, gap = 4, 2 // "[ " + " ]" is 4 columns; 2 columns of breathing room
	return prevX + len([]rune(prevLabel)) + brackets + gap
}

// trimLeadingSeparators strips leading path separators, for turning the
// suffix left after slicing a parent directory's length off a full path
// into a bare entry name regardless of platform separator.
func trimLeadingSeparators(s string) string {
	for len(s) > 0 && (s[0] == '/' || s[0] == '\\') {
		s = s[1:]
	}
	return s
}

// pathBar is the clickable current-path strip in newNavRow: it reads fp's
// path fresh every frame (so there is no separate "update the button text"
// call to wire through OnPathChanged) and colors itself from whether fp
// itself has focus, not the bar's own — Button's IsFocused only reflects
// the Tab-focus of the (deliberately non-focusable) bar, never the pane it
// labels. It also fills its full stretched width with that color, which is
// what actually reads as an input strip spanning the row rather than a
// small button floating in a stretch of empty chrome — a plain Button's
// DrawTextBounded only paints the exact cells its own text occupies, not
// whatever extra width a Flex weight handed it.
type pathBar struct {
	Graphite.BaseWidget
	app *Graphite.Application
	fp  *filepane.FilePane
}

// newPathBar creates a pathBar at (0, 0) with Width 0, i.e. "stretch to
// fill whatever space newNavRow's Flex offers it" (see BaseWidget's
// negative/zero-width convention).
func newPathBar(app *Graphite.Application, fp *filepane.FilePane) *pathBar {
	base := Graphite.NewBaseWidget(0, 0, 0, 1)
	base.IsFocusable = false
	return &pathBar{BaseWidget: base, app: app, fp: fp}
}

// DrawRelative implements Graphite.Widget.
func (p *pathBar) DrawRelative(c *Graphite.Canvas, offX, offY, pW, pH int) {
	p.BaseWidget.DrawRelative(c, offX, offY, pW, pH)
	// Bright navAccent while fp has focus (matching the nav buttons beside
	// it), a faint tint of the same hue otherwise — enough to still read as
	// "the same kind of control" without competing with the focused pane's
	// own accent-colored header/cursor.
	bg := navAccent.Darken(0.75)
	if p.fp.HasFocus() {
		bg = navAccent
	}
	fg := bg.ContrastText()
	for x := 0; x < p.LastW; x++ {
		c.DrawCell(p.AbsX+x, p.AbsY, " ", bg, fg)
	}
	c.DrawTextBounded(p.AbsX, p.AbsY, p.LastW, "[ "+p.fp.Path()+" ]", bg, fg)
}

// HandleEvent implements Graphite.Widget: a click opens the same
// go-to-folder prompt as before.
func (p *pathBar) HandleEvent(ev Graphite.Event) {
	if ev.Type != Graphite.EventMouseDown {
		return
	}
	promptGoTo(p.app, p.fp)
}

// promptGoTo is Diskette's cd-bar: a modal prompting for a path, rather
// than a persistent input row, precisely so it never becomes a third
// top-level focusable widget (see newNavButton).
func promptGoTo(app *Graphite.Application, fp *filepane.FilePane) {
	Graphite.ShowTextEditor(app, app.T(locales.KeyGotoFolderTitle), app.T(locales.KeyPath), fp.Path(), func(path string) {
		if _, err := fp.FS.Stat(context.Background(), path); err != nil {
			app.ShowMessage(app.T(locales.KeyErrorTitle), err.Error(), Graphite.BtnDanger)
			return
		}
		fp.SetPath(path)
	})
}

// promptChooseRoot opens a modal listing this platform's filesystem roots
// (drive letters on Windows, "/" plus mounted volumes on macOS, "/" plus
// real mount points on Linux — see internal/roots) and navigates fp to
// whichever one is picked.
func promptChooseRoot(app *Graphite.Application, fp *filepane.FilePane) {
	items := roots.List()
	height := len(items) + 6
	if height > 18 {
		height = 18
	}
	mod := Graphite.NewWindow(34, height, app.T(locales.KeyChooseRootTitle))
	mod.AddWidget(Graphite.NewListBox(2, 1, -2, -3, items, func(_ int, item string) {
		app.CloseModal()
		fp.SetPath(item)
	}))
	mod.AddWidget(Graphite.NewButton(2, -2, app.T(locales.KeyCancel), Graphite.BtnDefault, func() {
		app.CloseModal()
	}))
	app.SetModal(mod)
}

// doRename implements F2: rename the entry under the cursor.
func doRename(app *Graphite.Application, fp *filepane.FilePane) {
	entry, ok := fp.Selected()
	if !ok {
		return
	}
	Graphite.ShowTextEditor(app, app.T(locales.KeyRenameTitle), app.T(locales.KeyNewName), entry.Name, func(newName string) {
		ctx := context.Background()
		oldPath := fp.FS.Join(fp.Path(), entry.Name)
		newPath := fp.FS.Join(fp.Path(), newName)
		if err := fp.FS.Rename(ctx, oldPath, newPath); err != nil {
			app.ShowMessage(app.T(locales.KeyErrorTitle), err.Error(), Graphite.BtnDanger)
		}
		fp.Reload()
	})
}

// doNewFile implements "New File": create an empty file inside fp's
// current directory and select it, leaving the user to open it however
// they like. Refuses to proceed if the name already exists, since
// fp.FS.Create truncates unconditionally (unlike Mkdir, which already
// errors on its own for an existing path) — silently emptying an
// existing file just because its name was reused would be a real way to
// lose data.
func doNewFile(app *Graphite.Application, fp *filepane.FilePane) {
	Graphite.ShowTextEditor(app, app.T(locales.KeyNewFileTitle), app.T(locales.KeyName), "", func(name string) {
		ctx := context.Background()
		path := fp.FS.Join(fp.Path(), name)
		if _, err := fp.FS.Stat(ctx, path); err == nil {
			app.ShowMessage(app.T(locales.KeyErrorTitle), app.T(locales.KeyAlreadyExists, name), Graphite.BtnDanger)
			return
		}
		w, err := fp.FS.Create(ctx, path)
		if err != nil {
			app.ShowMessage(app.T(locales.KeyErrorTitle), err.Error(), Graphite.BtnDanger)
			return
		}
		if err := w.Close(); err != nil {
			app.ShowMessage(app.T(locales.KeyErrorTitle), err.Error(), Graphite.BtnDanger)
			return
		}
		fp.Reload()
		fp.SelectByName(name)
	})
}

// doMkdir implements F7: create a new directory inside fp's current path.
func doMkdir(app *Graphite.Application, fp *filepane.FilePane) {
	Graphite.ShowTextEditor(app, app.T(locales.KeyNewFolderTitle), app.T(locales.KeyName), "", func(name string) {
		if err := fp.FS.Mkdir(context.Background(), fp.FS.Join(fp.Path(), name)); err != nil {
			app.ShowMessage(app.T(locales.KeyErrorTitle), err.Error(), Graphite.BtnDanger)
		}
		fp.Reload()
	})
}

// doDelete implements F8: delete the tagged entries, or the entry under
// the cursor if nothing is tagged, after a Yes/No confirmation. The actual
// filesystem work runs on a background goroutine so the UI keeps
// rendering; the reload/error report hops back to the main loop via
// Application.Invoke.
func doDelete(app *Graphite.Application, fp *filepane.FilePane) {
	paths := fp.SelectionPaths()
	if len(paths) == 0 {
		return
	}
	msg := app.T(locales.KeyDeleteMessage, len(paths))
	Graphite.ShowConfirm(app, app.T(locales.KeyDeleteTitle), msg, Graphite.BtnDanger, func() {
		go func() {
			ctx := context.Background()
			var firstErr error
			for _, p := range paths {
				if err := copyengine.RemoveAll(ctx, fp.FS, p); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			app.Invoke(func() {
				fp.Reload()
				if firstErr != nil {
					app.ShowMessage(app.T(locales.KeyErrorTitle), firstErr.Error(), Graphite.BtnDanger)
				}
			})
		}()
	})
}

// doCopyOrMove implements F5/F6: copy or move src's selection into dst's
// current directory, showing a progress modal and, on collision, a
// conflict modal with an "apply to all" option — copyengine.Run does the
// actual work on a background goroutine, hopping back to the main loop via
// Application.Invoke for anything that touches widget state.
func doCopyOrMove(app *Graphite.Application, src, dst *filepane.FilePane, move bool) {
	paths := src.SelectionPaths()
	if len(paths) == 0 {
		return
	}

	title := app.T(locales.KeyCopyTitle)
	if move {
		title = app.T(locales.KeyMoveTitle)
	}

	ctx, cancel := context.WithCancel(context.Background())

	progressLbl := Graphite.NewLabel(2, 2, "")
	bar := Graphite.NewProgressBar(2, 4, 40, "")
	mod := Graphite.NewWindow(50, 9, title)
	mod.AddWidget(progressLbl)
	mod.AddWidget(bar)
	// Y=-2 (2 rows above the bottom of the content area), not a fixed
	// row number: Window's default PaddingY shrinks the content area a
	// child's Y is resolved against, so a small fixed window height like
	// this one's can silently place a button past the actual content
	// bounds — BaseWidget.DrawRelative's parent-bounds clamp then caps
	// its LastH at 0, leaving it focusable and Enter-triggerable but
	// never clickable (HitTest requires LastH > 0). See graphite's
	// ShowConfirm for the same fix and fuller reasoning.
	mod.AddWidget(Graphite.NewButton(2, -2, app.T(locales.KeyCancel), Graphite.BtnDefault, func() {
		cancel()
	}))
	app.SetModal(mod)

	task := copyengine.Task{
		SrcFS:    src.FS,
		DstFS:    dst.FS,
		SrcPaths: paths,
		DstDir:   dst.Path(),
		Move:     move,
	}

	onProgress := func(p copyengine.Progress) {
		app.Invoke(func() {
			progressLbl.SetText(p.CurrentPath)
			if p.FilesTotal > 0 {
				bar.SetProgress(float32(p.FilesDone) * 100 / float32(p.FilesTotal))
			}
		})
	}

	onConflict := func(c copyengine.Conflict) copyengine.Resolution {
		answer := make(chan copyengine.Resolution, 1)
		app.Invoke(func() {
			showConflictModal(app, c, func(r copyengine.Resolution) { answer <- r })
		})
		return <-answer
	}

	go func() {
		_, runErr := copyengine.Run(ctx, task, onProgress, onConflict)
		app.Invoke(func() {
			app.CloseModal() // the progress modal
			src.Reload()
			dst.Reload()
			if runErr != nil && runErr != copyengine.ErrCanceledByUser && ctx.Err() == nil {
				msg := runErr.Error()
				if errors.Is(runErr, copyengine.ErrDestinationInsideSource) {
					msg = app.T(locales.KeyErrDestinationInsideSrc)
				}
				app.ShowMessage(app.T(locales.KeyErrorTitle), msg, Graphite.BtnDanger)
			}
		})
	}()
}

// doZip implements the gutter's Zip button: packs src's tagged selection
// (or the entry under its cursor, matching every other selection-based
// action — see FilePane.SelectionPaths) into a single .zip file written
// into dst's directory, the same source-toward-destination direction
// Copy and Move already use. Prompts for the archive's name first,
// defaulting to "<name>.zip" for a single-item selection (matching the
// convention most file managers use) or "Archive.zip" for several, and
// confirms before overwriting an existing file of that name.
func doZip(app *Graphite.Application, src, dst *filepane.FilePane) {
	paths := src.SelectionPaths()
	if len(paths) == 0 {
		return
	}

	defaultName := "Archive.zip"
	if count, _, _ := src.TaggedSummary(); count == 0 {
		if e, ok := src.Selected(); ok {
			defaultName = e.Name + ".zip"
		}
	}

	Graphite.ShowTextEditor(app, app.T(locales.KeyZipTitle), app.T(locales.KeyArchiveName), defaultName, func(name string) {
		if name == "" {
			return
		}
		if !strings.HasSuffix(strings.ToLower(name), ".zip") {
			name += ".zip"
		}
		archivePath := dst.FS.Join(dst.Path(), name)

		start := func() {
			runArchiveTask(app, app.T(locales.KeyZipTitle), src, dst, func(ctx context.Context, onProgress archiveengine.ProgressFunc) error {
				return archiveengine.CreateZip(ctx, src.FS, paths, dst.FS, archivePath, onProgress)
			})
		}
		if _, err := dst.FS.Stat(context.Background(), archivePath); err == nil {
			Graphite.ShowConfirm(app, app.T(locales.KeyZipTitle), app.T(locales.KeyArchiveOverwrite, name), Graphite.BtnDanger, start)
			return
		}
		start()
	})
}

// doUnzip implements the gutter's Unzip button: extracts every archive
// tagged in src (or the one under its cursor) into dst's directory,
// auto-detecting each one's format from its extension — see
// archiveengine.DetectFormat for the full list (.zip, .tar, .tar.gz/
// .tgz, .tar.bz2/.tbz2/.tbz). Rejects the whole batch up front if
// anything selected isn't a recognized archive, rather than extracting
// some and failing partway through on an unrelated file.
func doUnzip(app *Graphite.Application, src, dst *filepane.FilePane) {
	paths := src.SelectionPaths()
	if len(paths) == 0 {
		return
	}

	for _, p := range paths {
		if archiveengine.DetectFormat(p) == archiveengine.FormatUnknown {
			app.ShowMessage(app.T(locales.KeyErrorTitle), app.T(locales.KeyNotAnArchive, p), Graphite.BtnDanger)
			return
		}
	}

	runArchiveTask(app, app.T(locales.KeyUnzipTitle), src, dst, func(ctx context.Context, onProgress archiveengine.ProgressFunc) error {
		for _, p := range paths {
			if err := archiveengine.ExtractArchive(ctx, src.FS, p, dst.FS, dst.Path(), onProgress); err != nil {
				return err
			}
		}
		return nil
	})
}

// runArchiveTask runs fn (CreateZip or a run of ExtractArchive calls) on
// a background goroutine behind a Cancel-able progress modal, then
// reloads both panes and reports any error — the same shape
// doCopyOrMove's own goroutine/modal wiring uses, generalized so Zip and
// Unzip can share it instead of duplicating it twice more.
func runArchiveTask(app *Graphite.Application, title string, src, dst *filepane.FilePane, fn func(ctx context.Context, onProgress archiveengine.ProgressFunc) error) {
	ctx, cancel := context.WithCancel(context.Background())

	progressLbl := Graphite.NewLabel(2, 2, "")
	bar := Graphite.NewProgressBar(2, 4, 40, "")
	mod := Graphite.NewWindow(50, 9, title)
	mod.AddWidget(progressLbl)
	mod.AddWidget(bar)
	mod.AddWidget(Graphite.NewButton(2, -2, app.T(locales.KeyCancel), Graphite.BtnDefault, func() {
		cancel()
	}))
	app.SetModal(mod)

	onProgress := func(p archiveengine.Progress) {
		app.Invoke(func() {
			progressLbl.SetText(p.CurrentPath)
			if p.Total > 0 {
				bar.SetProgress(float32(p.Done) * 100 / float32(p.Total))
			}
		})
	}

	go func() {
		runErr := fn(ctx, onProgress)
		app.Invoke(func() {
			app.CloseModal() // the progress modal
			src.Reload()
			dst.Reload()
			if runErr != nil && ctx.Err() == nil {
				app.ShowMessage(app.T(locales.KeyErrorTitle), runErr.Error(), Graphite.BtnDanger)
			}
		})
	}()
}

// showConflictModal asks how to resolve one destination collision,
// calling respond exactly once with the chosen Resolution. It runs on the
// main loop (the caller is expected to reach it via Application.Invoke,
// since copyengine.Run calls its ResolveFunc from a background goroutine).
func showConflictModal(app *Graphite.Application, c copyengine.Conflict, respond func(copyengine.Resolution)) {
	// Width 84, not the original 56: wide enough for all four buttons'
	// Ukrainian labels (the longest translation of this row across
	// en/uk/ru/nl) laid out via nextButtonX, accounting for Window's own
	// default PaddingX (4 on each side, so only width-8 is actually
	// available to children) — without that margin the row still ran a
	// few columns past the window's own right edge.
	mod := Graphite.NewWindow(84, 12, app.T(locales.KeyConflictTitle))
	mod.AddWidget(Graphite.NewLabel(2, 1, app.T(locales.KeyConflictExists)))
	mod.AddWidget(Graphite.NewLabel(2, 2, c.Path))

	applyAll := Graphite.NewCheckbox(2, 4, app.T(locales.KeyConflictApplyAll), false)
	mod.AddWidget(applyAll)

	resolve := func(action copyengine.ConflictAction) {
		app.CloseModal()
		respond(copyengine.Resolution{Action: action, ForAll: applyAll.Checked})
	}

	overwriteLabel := app.T(locales.KeyConflictOverwrite)
	skipLabel := app.T(locales.KeyConflictSkip)
	renameLabel := app.T(locales.KeyConflictRename)
	overwriteX := 2
	skipX := nextButtonX(overwriteX, overwriteLabel)
	renameX := nextButtonX(skipX, skipLabel)
	cancelX := nextButtonX(renameX, renameLabel)

	mod.AddWidget(Graphite.NewButton(overwriteX, 6, overwriteLabel, Graphite.BtnDanger, func() {
		resolve(copyengine.Overwrite)
	}))
	mod.AddWidget(Graphite.NewButton(skipX, 6, skipLabel, Graphite.BtnDefault, func() {
		resolve(copyengine.Skip)
	}))
	mod.AddWidget(Graphite.NewButton(renameX, 6, renameLabel, Graphite.BtnDefault, func() {
		app.CloseModal() // this conflict modal
		Graphite.ShowTextEditor(app, app.T(locales.KeyRenameTitle), app.T(locales.KeyNewName), "", func(newName string) {
			respond(copyengine.Resolution{Action: copyengine.Rename, NewName: newName, ForAll: applyAll.Checked})
		})
	}))
	mod.AddWidget(Graphite.NewButton(cancelX, 6, app.T(locales.KeyCancel), Graphite.BtnDefault, func() {
		resolve(copyengine.Cancel)
	}))

	app.SetModal(mod)
}

// showManageTabs implements the Tab menu's "Manage Tabs...": both panes'
// tabs side by side, each in its own ListBox (● prefix marks a pinned
// one; selecting a row toggles its pin — a plain click being the fastest
// path to "protect this from being reset on restart" rather than needing
// a separate button per row), plus a small add/close toolbar under each.
func showManageTabs(app *Graphite.Application, left, right *paneTabs) {
	// Width 84, not the original 74: that width left the right column's
	// own add/close row (starting at the same fixed x=38 as the English
	// version) with zero margin before Window's own PaddingX-trimmed
	// right edge — enough for "Close" but not "Закрити"/"Sluiten".
	mod := Graphite.NewWindow(84, 18, app.T(locales.KeyManageTabsTitle))

	mod.AddWidget(Graphite.NewLabel(2, 1, app.T(locales.KeyManageTabsLeft)))
	mod.AddWidget(Graphite.NewLabel(38, 1, app.T(locales.KeyManageTabsRight)))

	// Height -6, not a fixed number: like every other button row anchored
	// from the bottom in this project (see doCopyOrMove's progress modal),
	// a fixed row number here would silently drift into the add/close
	// toolbar row below once padding is accounted for — this stops
	// exactly 2 rows above it instead, regardless of the window's exact
	// content-area height.
	leftList := Graphite.NewListBox(2, 2, 34, -6, tabListLabels(left), nil)
	rightList := Graphite.NewListBox(38, 2, 34, -6, tabListLabels(right), nil)
	mod.AddWidget(leftList)
	mod.AddWidget(rightList)

	refresh := func() {
		leftList.Items = tabListLabels(left)
		rightList.Items = tabListLabels(right)
	}
	leftList.OnSelect = func(idx int, _ string) { left.group.TogglePin(idx); refresh() }
	rightList.OnSelect = func(idx int, _ string) { right.group.TogglePin(idx); refresh() }

	// Y=-4: one row above Done (-2), the same bottom-anchored convention,
	// so the two rows never collide regardless of the window's exact
	// content-area height (see graphite's own dialogs.go for the bug this
	// avoids: a fixed positive row number silently overlapping a
	// bottom-anchored one once PaddingY is accounted for).
	addRow := func(x int, p *paneTabs, list *Graphite.ListBox) {
		mod.AddWidget(Graphite.NewButton(x, -4, app.T(locales.KeyManageTabsAddFiles), Graphite.BtnDefault, func() {
			path := mustGetwd()
			if fp, ok := p.ActiveFilePane(); ok {
				path = fp.Path()
			}
			p.AddFileList(path)
			refresh()
		}))
		mod.AddWidget(Graphite.NewButton(x+11, -4, app.T(locales.KeyManageTabsAddTerm), Graphite.BtnDefault, func() {
			if err := p.AddTerminal(); err != nil {
				app.ShowMessage(app.T(locales.KeyErrorTitle), err.Error(), Graphite.BtnDanger)
				return
			}
			refresh()
		}))
		mod.AddWidget(Graphite.NewButton(x+21, -4, app.T(locales.KeyManageTabsCloseTab), Graphite.BtnDanger, func() {
			p.CloseTabAt(list.Selected)
			refresh()
		}))
	}
	addRow(2, left, leftList)
	addRow(38, right, rightList)

	mod.AddWidget(Graphite.NewButton(2, -2, app.T(locales.KeyManageTabsDone), Graphite.BtnDefault, func() {
		app.CloseModal()
	}))

	app.SetModal(mod)
}

// tabListLabels renders p's tabs for showManageTabs' ListBox: a ● prefix
// for a pinned tab, the active tab's name in brackets so it's visible
// which one you're currently looking at.
func tabListLabels(p *paneTabs) []string {
	labels := make([]string, len(p.group.Tabs))
	for i, t := range p.group.Tabs {
		name := t.Name
		if i == p.group.Active {
			name = "[" + name + "]"
		}
		if t.Pinned {
			labels[i] = "● " + name
		} else {
			labels[i] = "  " + name
		}
	}
	return labels
}

// restoreSavedTabs seeds left/right from tabs.Load()'s pinned tabs, or —
// on any load error (nothing saved yet, a fresh install, a corrupt file)
// or a side with no saved tabs — falls back to the one thing every
// version of diskette before tabs existed already did: a single FileList
// tab at start.
func restoreSavedTabs(left, right *paneTabs, start string) {
	state, err := tabs.Load()
	if err != nil {
		left.AddFileList(start)
		right.AddFileList(start)
		return
	}
	restoreSide(left, state.Left, state.LeftActive, start)
	restoreSide(right, state.Right, state.RightActive, start)
}

func restoreSide(p *paneTabs, saved []tabs.SavedTab, active int, start string) {
	pendingRemotes := 0
	for _, st := range saved {
		switch st.Kind {
		case tabs.Terminal:
			if err := p.AddTerminal(); err == nil {
				p.group.Tabs[len(p.group.Tabs)-1].Pinned = true
				p.group.Tabs[len(p.group.Tabs)-1].Name = st.Name
			}
		case tabs.Remote:
			// No stored password to reconnect with — a pre-filled
			// Connect dialog per the project owner's own choice, not an
			// automatic reconnect. Nothing is added to p.group.Tabs
			// until (and unless) the user finishes it; showConnectDialog
			// itself pins the resulting tab and restores its saved name
			// via onConnected, since AddRemote alone doesn't know this
			// is a restore rather than a fresh connection.
			if st.Remote == nil {
				continue
			}
			pendingRemotes++
			meta, name := *st.Remote, st.Name
			showConnectDialog(p.app, p, &meta, "/", func() {
				p.group.Tabs[len(p.group.Tabs)-1].Pinned = true
				p.group.Tabs[len(p.group.Tabs)-1].Name = name
			})
		default:
			path := st.Path
			if _, err := os.Stat(path); err != nil {
				path = start // the saved directory no longer exists
			}
			p.AddFileList(path)
			p.group.Tabs[len(p.group.Tabs)-1].Pinned = true
			p.group.Tabs[len(p.group.Tabs)-1].Name = st.Name
		}
	}
	if len(p.group.Tabs) == 0 && pendingRemotes == 0 {
		p.AddFileList(start)
		return
	}
	p.group.SetActive(active)
}

// saveTabs writes every pinned tab (on either pane) to disk, so pinning is
// genuinely "protected from reset on restart" — an unpinned tab is
// deliberately not saved. A write failure (a read-only config directory,
// say) is reported rather than silently losing the user's pins.
func saveTabs(left, right *paneTabs) {
	state := tabs.SavedState{
		Left:        pinnedOf(left),
		LeftActive:  left.group.Active,
		Right:       pinnedOf(right),
		RightActive: right.group.Active,
		Locale:      string(left.app.Locale()),
	}
	tabs.Save(state) // best-effort: a failed save here shouldn't block quitting
}

func pinnedOf(p *paneTabs) []tabs.SavedTab {
	var out []tabs.SavedTab
	for _, t := range p.group.Tabs {
		if !t.Pinned {
			continue
		}
		content, ok := t.Widget.(tabContent)
		if !ok {
			continue
		}
		st := tabs.SavedTab{Kind: t.Kind, Name: t.Name, Pinned: true}
		if content.remote != nil {
			meta := content.remote.meta
			st.Remote = &meta
		} else if content.filePane != nil {
			st.Path = content.filePane.Path()
		}
		out = append(out, st)
	}
	return out
}
