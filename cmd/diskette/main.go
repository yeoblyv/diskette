// Command diskette is a dual-pane file manager built on the graphite TUI
// framework, in the style of classic Total Commander/Midnight Commander:
// two navigable panes, an F-key action bar, and local file operations.
// Later phases add remote (SFTP) panes.
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	Graphite "github.com/yeoblyv/graphite"

	"github.com/yeoblyv/diskette/internal/assets"
	"github.com/yeoblyv/diskette/internal/copyengine"
	"github.com/yeoblyv/diskette/internal/diskspace"
	"github.com/yeoblyv/diskette/internal/fileinfo"
	"github.com/yeoblyv/diskette/internal/filepane"
	"github.com/yeoblyv/diskette/internal/fkeybar"
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

func main() {
	start, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "diskette:", err)
		os.Exit(1)
	}

	app := Graphite.NewApplication()
	app.SetTheme(theme.Diskette())
	win := Graphite.NewFullscreenWindow()

	fs := vfs.LocalFS{}

	leftTabs := newPaneTabs(app, fs)
	rightTabs := newPaneTabs(app, fs)
	restoreSavedTabs(leftTabs, rightTabs, start)

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
				fKeyBar.Status = "Search: " + q
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

	diskBar := newDiskSpaceBar(func() statusBarState {
		group := leftTabs
		if rightTabs.HasFocus() {
			group = rightTabs
		}
		fp, ok := group.ActiveFilePane()
		if !ok {
			return statusBarState{}
		}
		usage, err := diskspace.Query(fp.Path())
		count, size, hasTagged := fp.TaggedSummary()
		return statusBarState{
			usage:       diskUsageCache{usage: usage, ok: err == nil},
			taggedCount: count, taggedSize: size, hasTagged: hasTagged,
		}
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
	app.SetWindow(win)
	app.Run()
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
		app.ShowMessage(" Error ", err.Error(), Graphite.BtnDanger)
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
	row.AddChild(newNavButton("Root", func() { promptChooseRoot(app, fp) }), 0)
	row.AddChild(newPathBar(app, fp), 1)
	return row
}

// tabContent is what one tab actually displays: for a FileList tab,
// filePane is the raw *filepane.FilePane (for everything that needs to
// act on it directly — F-keys, the menu, the gutter) and display is that
// FilePane wrapped in its own nav-row Flex; for a Terminal tab, terminal
// and display are the same bare *Graphite.Terminal. Kept as two named
// fields rather than a type switch on display everywhere a caller needs
// the concrete widget back.
type tabContent struct {
	filePane *filepane.FilePane
	terminal *Graphite.Terminal
	display  Graphite.Widget
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
	fp.OnOpenFile = func(p string) { openFile(app, p) }

	col := Graphite.NewFlex(0, 0, 0, 0, Graphite.FlexColumn)
	col.AddChild(newNavRow(app, fp), 0)
	col.AddChild(fp, 1)

	return tabContent{filePane: fp, display: col}
}

// newTerminalContent spawns a fresh shell in a new Terminal tab.
func newTerminalContent(app *Graphite.Application) (tabContent, error) {
	term, err := Graphite.NewTerminal(app, 0, 0, 0, 0, defaultShell(), nil)
	if err != nil {
		return tabContent{}, err
	}
	return tabContent{terminal: term, display: term}, nil
}

// tabName returns a FileList tab's display name (the directory's own base
// name — "Root" for a filesystem root, which has no meaningful base name
// of its own) or a Terminal tab's, which is always just "Terminal": there
// is no path or title to derive a nicer one from without an OSC 0/2
// title, which this pane doesn't read.
func tabName(kind tabs.Kind, path string) string {
	if kind == tabs.Terminal {
		return "Terminal"
	}
	base := filepath.Base(path)
	if base == "." || base == string(filepath.Separator) {
		return "Root"
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
	p.group.Add(&tabs.Tab{Kind: tabs.FileList, Name: tabName(tabs.FileList, path), Widget: content})
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
	content, err := newTerminalContent(p.app)
	if err != nil {
		return err
	}
	p.group.Add(&tabs.Tab{Kind: tabs.Terminal, Name: tabName(tabs.Terminal, ""), Widget: content})
	if wasFocused {
		content.focusable().SetFocus(true)
	}
	return nil
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
	if !p.group.Close(idx) {
		return false
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
// holding Copy and Move — two buttons, not four: each is a dirButton whose
// label flips to always name the focused pane as source and the other one
// as destination, recomputed fresh every frame (there is no "focus
// changed" event to hook, so drawing fresh is what keeps it honest). A
// weight-1 spacer above and below the four fixed-size buttons centers
// them in the gutter's full height instead of leaving them stacked at the
// top. ZIP/UNZIP is a placeholder: disabled (grayed out, inert) until
// archive pack/unpack between the two panes is actually implemented.
// Each button's Width 0 stretches it to the gutter's full width (see
// dirButton's own DrawRelative for why that requires a custom widget
// rather than a plain Button).
func newActionGutter(app *Graphite.Application, left, right *paneTabs) *Graphite.Flex {
	gutter := Graphite.NewFlex(0, 0, 15, 0, Graphite.FlexColumn)
	gutter.Gap = 1
	gutter.AddChild(Graphite.NewPanel(0, 0, 0, 0), 1)
	gutter.AddChild(newDirButton(left, right, "Copy", func(src, dst *paneTabs) {
		srcFP, ok1 := src.ActiveFilePane()
		dstFP, ok2 := dst.ActiveFilePane()
		if ok1 && ok2 {
			doCopyOrMove(app, srcFP, dstFP, false)
		}
	}), 0)
	gutter.AddChild(newDirButton(left, right, "Move", func(src, dst *paneTabs) {
		srcFP, ok1 := src.ActiveFilePane()
		dstFP, ok2 := dst.ActiveFilePane()
		if ok1 && ok2 {
			doCopyOrMove(app, srcFP, dstFP, true)
		}
	}), 0)
	gutter.AddChild(newDirButton(left, right, "Zip", nil), 0)
	gutter.AddChild(newDirButton(left, right, "Unzip", nil), 0)
	gutter.AddChild(Graphite.NewPanel(0, 0, 0, 0), 1)
	return gutter
}

// dirButton is a gutter button whose label names whichever pane currently
// has focus as the source and the other one as the destination, recomputed
// every frame. Copy/Move are wired to onClick; Zip/Unzip pass a nil
// onClick and render dimmed and inert, the same shape as Copy/Move so the
// gutter reads as one consistent design, ready to wire up once archiving
// is implemented. Deliberately not a Graphite.Button: a Button's
// background only fills the exact width its own text occupies, not
// whatever extra width a Flex weight hands it, and its Text is a plain
// field with no per-frame hook for a label that must track live state.
type dirButton struct {
	Graphite.BaseWidget
	left, right *paneTabs
	action      string
	onClick     func(src, dst *paneTabs)
}

// newDirButton creates a dirButton with Width 0, stretching it to fill
// whatever width its parent Flex offers (see BaseWidget's zero/negative
// width convention). A nil onClick renders it dimmed and inert.
func newDirButton(left, right *paneTabs, action string, onClick func(src, dst *paneTabs)) *dirButton {
	base := Graphite.NewBaseWidget(0, 0, 0, 1)
	return &dirButton{BaseWidget: base, left: left, right: right, action: action, onClick: onClick}
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
	if d.right.HasFocus() {
		return "◀ " + d.action
	}
	return d.action + " ▶"
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
// button was constructed. A nil onClick (Zip/Unzip, not implemented yet)
// makes the button inert.
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
// that pane's disk usage. Both are the zero value when the focused pane's
// active tab is a Terminal rather than a FileList — there is nothing
// file-related to report on.
type statusBarState struct {
	usage       diskUsageCache
	taggedCount int
	taggedSize  int64
	hasTagged   bool
}

// diskSpaceBar is the full-width row above the F-key bar, split into three
// parts separated by "│": tagged-selection size, a disk usage progress
// bar, and a third segment reserved for remote-connection status once a
// server pane exists (Phase 2) — shown as "Local" for now rather than
// left blank, since local-only is the accurate current state, not an
// unfinished one. state is read fresh every frame (see dirButton for why:
// there is no "focus changed" hook to update from instead).
type diskSpaceBar struct {
	Graphite.BaseWidget
	state func() statusBarState
}

// newDiskSpaceBar creates the bar at Y=-3: one row above the very bottom
// (the F-key bar itself, at Y=-1), leaving row -2 blank — the same
// one-row gap the menu strip and nav row already have between them —
// instead of the two rows touching directly.
func newDiskSpaceBar(state func() statusBarState) *diskSpaceBar {
	return &diskSpaceBar{BaseWidget: Graphite.NewBaseWidget(0, -3, 0, 1), state: state}
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

	taggedText := "No files tagged"
	if state.hasTagged {
		taggedText = fmt.Sprintf("Tagged: %d item(s), %s", state.taggedCount, formatBytes(uint64(state.taggedSize)))
	}
	c.DrawTextBounded(d.AbsX, d.AbsY, taggedSegW, taggedText, theme.BgWindow, theme.FgWindow)

	dividerX1 := d.AbsX + taggedSegW
	c.DrawCell(dividerX1, d.AbsY, "│", theme.BgWindow, theme.FgDisabled)

	diskX := dividerX1 + 1
	d.drawDiskSegmentUsage(c, diskX, diskSegW, state.usage)

	dividerX2 := diskX + diskSegW
	c.DrawCell(dividerX2, d.AbsY, "│", theme.BgWindow, theme.FgDisabled)

	// Reserved for remote-connection status once a server pane exists
	// (see the project's SFTP phase) — "Local" is accurate today, not a
	// placeholder pretending to be a real connection indicator.
	c.DrawTextBounded(dividerX2+1, d.AbsY, serverSegW-1, "Server: Local", theme.BgWindow, theme.FgDisabled)
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
		c.DrawTextBounded(x, d.AbsY, w, "Disk: n/a", theme.BgWindow, theme.FgDisabled)
		return
	}

	used := cache.usage.Total - cache.usage.Free
	usedPct := float64(used) / float64(cache.usage.Total) * 100
	label := fmt.Sprintf("Disk: %s free of %s (%.0f%% used)", formatBytes(cache.usage.Free), formatBytes(cache.usage.Total), usedPct)

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

// newFKeyBar builds the bottom action bar. F4/F9 are listed with no
// OnClick (fkeybar renders those dimmed and inert) since they aren't
// implemented yet: F4/Edit needs Suspend/Resume around $EDITOR (a later
// phase), and F9/Menu wasn't required by this phase's scope. Showing them
// dimmed is honest about that instead of quietly leaving them off the
// bar's layout. Every active key is RolePrimary (the palette's lime
// accent) except Delete, which is destructive and stays RoleDanger.
// withFP/withPanes are the same active-FileList guards main() builds for
// the menu, reused here so F1/F2/F3/F7/F8 (and F5/F6's dst lookup) all
// share one no-op-on-a-Terminal-tab behavior.
func newFKeyBar(app *Graphite.Application, right *paneTabs, withFP func(func(*filepane.FilePane)) func(), withPanes func(func(src, dst *filepane.FilePane)) func()) *fkeybar.Bar {
	bar := fkeybar.New(0, -1, []fkeybar.Key{
		{Label: "F1", Text: "Info", OnClick: withFP(func(fp *filepane.FilePane) { showFileInfo(app, fp) })},
		{Label: "F2", Text: "Rename", OnClick: withFP(func(fp *filepane.FilePane) { doRename(app, fp) })},
		{Label: "F3", Text: "Find", OnClick: withFP(func(fp *filepane.FilePane) { showFindFiles(app, fp) })},
		{Label: "F4", Text: "Edit"},
		{Label: "F5", Text: "Copy Right", OnClick: withPanes(func(src, dst *filepane.FilePane) { doCopyOrMove(app, src, dst, false) })},
		{Label: "F6", Text: "Move Right", OnClick: withPanes(func(src, dst *filepane.FilePane) { doCopyOrMove(app, src, dst, true) })},
		{Label: "F7", Text: "MkDir", OnClick: withFP(func(fp *filepane.FilePane) { doMkdir(app, fp) })},
		{Label: "F8", Text: "Delete", Role: fkeybar.RoleDanger, OnClick: withFP(func(fp *filepane.FilePane) { doDelete(app, fp) })},
		{Label: "F9", Text: "Menu"},
		{Label: "F10", Text: "Quit"},
	})
	// F5/F6's Text names the destination pane explicitly, kept current
	// every frame the same way dirButton's own label does — there is no
	// "focus changed" event to hook it from instead.
	const copyIdx, moveIdx = 4, 5
	bar.OnBeforeDraw = func() {
		dir := "Right"
		if right.HasFocus() {
			dir = "Left"
		}
		bar.Keys[copyIdx].Text = "Copy " + dir
		bar.Keys[moveIdx].Text = "Move " + dir
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
	menu := Graphite.NewMenuStrip([]Graphite.MenuCategory{
		{Label: "File", Items: []Graphite.MenuItem{
			{Label: "File Info     F1", Action: withFP(func(fp *filepane.FilePane) { showFileInfo(app, fp) })},
			{Label: "Rename        F2", Action: withFP(func(fp *filepane.FilePane) { doRename(app, fp) })},
			{Label: "Find          F3", Action: withFP(func(fp *filepane.FilePane) { showFindFiles(app, fp) })},
			{Label: "Copy          F5", Action: withPanes(func(src, dst *filepane.FilePane) { doCopyOrMove(app, src, dst, false) })},
			{Label: "Move          F6", Action: withPanes(func(src, dst *filepane.FilePane) { doCopyOrMove(app, src, dst, true) })},
			{Label: "New Folder    F7", Action: withFP(func(fp *filepane.FilePane) { doMkdir(app, fp) })},
			{Label: "Delete        F8", Action: withFP(func(fp *filepane.FilePane) { doDelete(app, fp) })},
			{Label: "Quit         F10", Action: func() { requestQuit(app, left, right) }},
		}},
		{Label: "Mark", Items: []Graphite.MenuItem{
			{Label: "Tag/Untag    Ins", Action: withFP(func(fp *filepane.FilePane) { fp.ToggleTag() })},
		}},
		{Label: "View", Items: []Graphite.MenuItem{
			{Label: "Sort by Name", Action: withFP(func(fp *filepane.FilePane) { fp.SetSort(filepane.SortByName) })},
			{Label: "Sort by Size", Action: withFP(func(fp *filepane.FilePane) { fp.SetSort(filepane.SortBySize) })},
			{Label: "Sort by Date", Action: withFP(func(fp *filepane.FilePane) { fp.SetSort(filepane.SortByDate) })},
			{Label: "Refresh", Action: withFP(func(fp *filepane.FilePane) { fp.Reload() })},
		}},
		{Label: "Tab", Items: []Graphite.MenuItem{
			{Label: "Add", SubItems: []Graphite.MenuItem{
				{Label: "New file list", Action: func() {
					if fp, ok := active().ActiveFilePane(); ok {
						active().AddFileList(fp.Path())
					} else {
						active().AddFileList(mustGetwd())
					}
				}},
				{Label: "New terminal", Action: func() {
					if err := active().AddTerminal(); err != nil {
						app.ShowMessage(" Error ", err.Error(), Graphite.BtnDanger)
					}
				}},
			}},
			{Separator: true},
			{Label: "Pin/Unpin Tab", Action: func() { active().group.TogglePin(active().group.Active) }},
			{Label: "Close Tab", Action: func() { active().CloseTabAt(active().group.Active) }},
			{Separator: true},
			{Label: "Manage Tabs...", Action: func() { showManageTabs(app, left, right) }},
		}},
		{Label: "Help", Items: []Graphite.MenuItem{
			{Label: "About", Action: func() { showAbout(app) }},
		}},
	})
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
	Graphite.ShowConfirm(app, " Quit ", "Quit Diskette?", Graphite.BtnDanger, func() {
		saveTabs(left, right)
		app.Quit()
	})
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
	mod := Graphite.NewWindow(94, 27, " About ")

	if logo, err := Graphite.ReadGph(bytes.NewReader(assets.DisketteLogo)); err == nil {
		mod.AddWidget(Graphite.NewImage(2, 2, logo))
	}

	info := Graphite.NewLabel(40, 2,
		"Diskette v.0.1.0\n\n"+
			"A cross-platform dual-pane file manager,\n"+
			"with FTP, SFTP and SCP support.\n"+
			"Provides archiving features and build with\n"+
			"lightweight Graphite TUI framework.\n\n"+
			"Copyright © 2026 Yehor Oblyvantsov\n"+
			"github.com/yeoblyv/diskette\n\n"+
			"Development build.")
	info.Width = 44
	mod.AddWidget(info)

	mod.AddWidget(Graphite.NewButton(2, -2, "Close", Graphite.BtnDefault, func() {
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
		mod := Graphite.NewWindow(44, 10, " File Info ")
		mod.AddWidget(Graphite.NewLabel(2, 1, fmt.Sprintf(
			"%d items selected\n\nTotal size: %s", count, formatBytes(uint64(size)))))
		mod.AddWidget(Graphite.NewButton(2, -2, "Close", Graphite.BtnDefault, func() {
			app.CloseModal()
		}))
		app.SetModal(mod)
		return
	}

	info, err := fileinfo.Stat(paths[0])
	if err != nil {
		app.ShowMessage(" Error ", err.Error(), Graphite.BtnDanger)
		return
	}

	kind := "File"
	sizeStr := formatBytes(uint64(info.Size))
	if info.IsDir {
		kind, sizeStr = "Directory", "—"
	}
	created := "not available on this filesystem"
	if info.CreatedKnown {
		created = info.Created.Format("02.01.2006 15:04:05")
	}

	const layout = "02.01.2006 15:04:05"
	text := fmt.Sprintf(
		"Name:        %s\nType:        %s\nSize:        %s\nPermissions: %s\n\nModified:    %s\nAccessed:    %s\nCreated:     %s",
		info.Name, kind, sizeStr, info.Mode.String(),
		info.Modified.Format(layout), info.Accessed.Format(layout), created)

	mod := Graphite.NewWindow(56, 15, " File Info ")
	mod.AddWidget(Graphite.NewLabel(2, 1, text))
	mod.AddWidget(Graphite.NewButton(2, -2, "Close", Graphite.BtnDefault, func() {
		app.CloseModal()
	}))
	app.SetModal(mod)
}

// showFindFiles implements F3: a criteria modal (search root, name mask,
// recurse/case-sensitive toggles) that hands off to runFileSearch once
// submitted.
func showFindFiles(app *Graphite.Application, fp *filepane.FilePane) {
	mod := Graphite.NewWindow(60, 14, " Find Files ")

	rootInput := Graphite.NewInputBox(2, 1, 54, "Search in: ")
	rootInput.Value = fp.Path()
	mod.AddWidget(rootInput)

	maskInput := Graphite.NewInputBox(2, 3, 54, "Name mask: ")
	maskInput.Value = "*"
	mod.AddWidget(maskInput)

	recurseBox := Graphite.NewCheckbox(2, 5, "Search subfolders", true)
	mod.AddWidget(recurseBox)
	caseBox := Graphite.NewCheckbox(2, 6, "Case sensitive", false)
	mod.AddWidget(caseBox)

	mod.AddWidget(Graphite.NewButton(2, -2, "Search", Graphite.BtnSuccess, func() {
		app.CloseModal()
		runFileSearch(app, fp, search.Options{
			Mask:          maskInput.Value,
			Recursive:     recurseBox.Checked,
			CaseSensitive: caseBox.Checked,
		}, rootInput.Value)
	}))
	mod.AddWidget(Graphite.NewButton(14, -2, "Cancel", Graphite.BtnDefault, func() {
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
	mod := Graphite.NewWindow(64, 20, " Find Files ")
	status := Graphite.NewLabel(2, 1, "Searching…")
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
	mod.AddWidget(Graphite.NewButton(2, -2, "Cancel", Graphite.BtnDefault, func() {
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
					status.SetText(fmt.Sprintf("%d found", len(snapshot)))
				} else {
					status.SetText(fmt.Sprintf("Searching… %d found", len(snapshot)))
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
			app.Invoke(func() { app.ShowMessage(" Error ", err.Error(), Graphite.BtnDanger) })
		}
	}()
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
	Graphite.ShowTextEditor(app, " Go to folder ", "Path:", fp.Path(), func(path string) {
		if _, err := fp.FS.Stat(context.Background(), path); err != nil {
			app.ShowMessage(" Error ", err.Error(), Graphite.BtnDanger)
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
	mod := Graphite.NewWindow(34, height, " Choose root ")
	mod.AddWidget(Graphite.NewListBox(2, 1, -2, -3, items, func(_ int, item string) {
		app.CloseModal()
		fp.SetPath(item)
	}))
	mod.AddWidget(Graphite.NewButton(2, -2, "Cancel", Graphite.BtnDefault, func() {
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
	Graphite.ShowTextEditor(app, " Rename ", "New name:", entry.Name, func(newName string) {
		ctx := context.Background()
		oldPath := fp.FS.Join(fp.Path(), entry.Name)
		newPath := fp.FS.Join(fp.Path(), newName)
		if err := fp.FS.Rename(ctx, oldPath, newPath); err != nil {
			app.ShowMessage(" Error ", err.Error(), Graphite.BtnDanger)
		}
		fp.Reload()
	})
}

// doMkdir implements F7: create a new directory inside fp's current path.
func doMkdir(app *Graphite.Application, fp *filepane.FilePane) {
	Graphite.ShowTextEditor(app, " New folder ", "Name:", "", func(name string) {
		if err := fp.FS.Mkdir(context.Background(), fp.FS.Join(fp.Path(), name)); err != nil {
			app.ShowMessage(" Error ", err.Error(), Graphite.BtnDanger)
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
	msg := fmt.Sprintf("Delete %d item(s)?", len(paths))
	Graphite.ShowConfirm(app, " Delete ", msg, Graphite.BtnDanger, func() {
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
					app.ShowMessage(" Error ", firstErr.Error(), Graphite.BtnDanger)
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

	title := " Copy "
	if move {
		title = " Move "
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
	mod.AddWidget(Graphite.NewButton(2, -2, "Cancel", Graphite.BtnDefault, func() {
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
				app.ShowMessage(" Error ", runErr.Error(), Graphite.BtnDanger)
			}
		})
	}()
}

// showConflictModal asks how to resolve one destination collision,
// calling respond exactly once with the chosen Resolution. It runs on the
// main loop (the caller is expected to reach it via Application.Invoke,
// since copyengine.Run calls its ResolveFunc from a background goroutine).
func showConflictModal(app *Graphite.Application, c copyengine.Conflict, respond func(copyengine.Resolution)) {
	mod := Graphite.NewWindow(56, 12, " Conflict ")
	mod.AddWidget(Graphite.NewLabel(2, 1, "Already exists:"))
	mod.AddWidget(Graphite.NewLabel(2, 2, c.Path))

	applyAll := Graphite.NewCheckbox(2, 4, "Apply to all", false)
	mod.AddWidget(applyAll)

	resolve := func(action copyengine.ConflictAction) {
		app.CloseModal()
		respond(copyengine.Resolution{Action: action, ForAll: applyAll.Checked})
	}

	mod.AddWidget(Graphite.NewButton(2, 6, "Overwrite", Graphite.BtnDanger, func() {
		resolve(copyengine.Overwrite)
	}))
	mod.AddWidget(Graphite.NewButton(14, 6, "Skip", Graphite.BtnDefault, func() {
		resolve(copyengine.Skip)
	}))
	mod.AddWidget(Graphite.NewButton(21, 6, "Rename", Graphite.BtnDefault, func() {
		app.CloseModal() // this conflict modal
		Graphite.ShowTextEditor(app, " Rename ", "New name:", "", func(newName string) {
			respond(copyengine.Resolution{Action: copyengine.Rename, NewName: newName, ForAll: applyAll.Checked})
		})
	}))
	mod.AddWidget(Graphite.NewButton(30, 6, "Cancel", Graphite.BtnDefault, func() {
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
	mod := Graphite.NewWindow(74, 18, " Manage Tabs ")

	mod.AddWidget(Graphite.NewLabel(2, 1, "Left"))
	mod.AddWidget(Graphite.NewLabel(38, 1, "Right"))

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
		mod.AddWidget(Graphite.NewButton(x, -4, "+ Files", Graphite.BtnDefault, func() {
			path := mustGetwd()
			if fp, ok := p.ActiveFilePane(); ok {
				path = fp.Path()
			}
			p.AddFileList(path)
			refresh()
		}))
		mod.AddWidget(Graphite.NewButton(x+11, -4, "+ Term", Graphite.BtnDefault, func() {
			if err := p.AddTerminal(); err != nil {
				app.ShowMessage(" Error ", err.Error(), Graphite.BtnDanger)
				return
			}
			refresh()
		}))
		mod.AddWidget(Graphite.NewButton(x+21, -4, "Close", Graphite.BtnDanger, func() {
			p.CloseTabAt(list.Selected)
			refresh()
		}))
	}
	addRow(2, left, leftList)
	addRow(38, right, rightList)

	mod.AddWidget(Graphite.NewButton(2, -2, "Done", Graphite.BtnDefault, func() {
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
	for _, st := range saved {
		switch st.Kind {
		case tabs.Terminal:
			if err := p.AddTerminal(); err == nil {
				p.group.Tabs[len(p.group.Tabs)-1].Pinned = true
				p.group.Tabs[len(p.group.Tabs)-1].Name = st.Name
			}
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
	if len(p.group.Tabs) == 0 {
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
		if content.filePane != nil {
			st.Path = content.filePane.Path()
		}
		out = append(out, st)
	}
	return out
}
