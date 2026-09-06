// Command diskette is a dual-pane file manager built on the graphite TUI
// framework, in the style of classic Total Commander/Midnight Commander:
// two navigable panes, an F-key action bar, and local file operations.
// Later phases add remote (SFTP) panes.
package main

import (
	"context"
	"fmt"
	"os"

	Graphite "github.com/yeoblyv/graphite"

	"github.com/yeoblyv/diskette/internal/copyengine"
	"github.com/yeoblyv/diskette/internal/diskspace"
	"github.com/yeoblyv/diskette/internal/filepane"
	"github.com/yeoblyv/diskette/internal/fkeybar"
	"github.com/yeoblyv/diskette/internal/roots"
	"github.com/yeoblyv/diskette/internal/theme"
	"github.com/yeoblyv/diskette/internal/vfs"
)

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

	left := filepane.New(0, 0, 0, 0, fs, start)
	right := filepane.New(0, 0, 0, 0, fs, start)

	activePane := func() *filepane.FilePane {
		if right.HasFocus() {
			return right
		}
		return left
	}
	otherPane := func(p *filepane.FilePane) *filepane.FilePane {
		if p == left {
			return right
		}
		return left
	}

	fKeyBar := newFKeyBar(app, activePane, otherPane)

	refreshStatus := func() {
		if q := activePane().SearchQuery(); q != "" {
			fKeyBar.Status = "Search: " + q
			return
		}
		fKeyBar.Status = ""
	}
	left.OnSearchChanged = func(string) { refreshStatus() }
	right.OnSearchChanged = func(string) { refreshStatus() }
	refreshStatus()

	var diskUsage [2]diskUsageCache // indexed by [left, right]
	refreshDiskUsage := func(fp *filepane.FilePane, i int) {
		u, err := diskspace.Query(fp.Path())
		diskUsage[i] = diskUsageCache{usage: u, ok: err == nil}
	}

	leftNav, leftPathBtn := newNavRow(app, left)
	rightNav, rightPathBtn := newNavRow(app, right)
	left.OnPathChanged = func(path string) { setButtonText(leftPathBtn, path); refreshDiskUsage(left, 0) }
	right.OnPathChanged = func(path string) { setButtonText(rightPathBtn, path); refreshDiskUsage(right, 1) }
	refreshDiskUsage(left, 0)
	refreshDiskUsage(right, 1)

	leftCol := Graphite.NewFlex(0, 0, 0, 0, Graphite.FlexColumn)
	leftCol.AddChild(leftNav, 0)
	leftCol.AddChild(left, 1)

	rightCol := Graphite.NewFlex(0, 0, 0, 0, Graphite.FlexColumn)
	rightCol.AddChild(rightNav, 0)
	rightCol.AddChild(right, 1)

	mainRow := Graphite.NewFlex(0, 1, 0, -2, Graphite.FlexRow)
	mainRow.Gap = 1
	mainRow.AddChild(leftCol, 1)
	mainRow.AddChild(newActionGutter(app, left, right), 0)
	mainRow.AddChild(rightCol, 1)

	diskBar := newDiskSpaceBar(func() diskUsageCache {
		if right.HasFocus() {
			return diskUsage[1]
		}
		return diskUsage[0]
	})

	win.AddWidget(mainRow)
	win.AddWidget(diskBar)
	win.AddWidget(fKeyBar)
	// menu is added last so its open dropdown wins Window's hit-test
	// priority over the panes it visually overlaps — see newMenuStrip.
	win.AddWidget(newMenuStrip(app, activePane, otherPane))

	onFKey := func(source *filepane.FilePane) func(Graphite.KeyCode) {
		return func(key Graphite.KeyCode) {
			other := otherPane(source)
			switch key {
			case Graphite.KeyF2:
				doRename(app, source)
			case Graphite.KeyF5:
				doCopyOrMove(app, source, other, false)
			case Graphite.KeyF6:
				doCopyOrMove(app, source, other, true)
			case Graphite.KeyF7:
				doMkdir(app, source)
			case Graphite.KeyF8:
				doDelete(app, source)
			case Graphite.KeyF10:
				requestQuit(app)
			}
		}
	}
	left.OnFunctionKey = onFKey(left)
	right.OnFunctionKey = onFKey(right)

	app.SetOnQuitRequested(func() { requestQuit(app) })
	app.SetWindow(win)
	app.Run()
}

// newNavButton creates a small, non-focusable Button. It is deliberately
// not focusable (IsFocusable is forced back to false right after
// construction): the two FilePanes must stay the only two top-level
// focusable widgets in the window, so Tab keeps switching directly between
// them — see the project's architecture notes on this. Clicking it still
// works regardless of focusability, since Window delivers a mouse click to
// whatever it hit-tests under the pointer independently of the Tab order.
func newNavButton(label string, onClick func()) *Graphite.Button {
	btn := Graphite.NewButton(0, 0, label, Graphite.BtnDefault, onClick)
	btn.IsFocusable = false
	return btn
}

// newNavRow builds one pane's navigation strip — Back/Forward/Refresh, a
// root/drive picker, and the clickable current-path bar — as a Flex row so
// the path bar always fills exactly the space the fixed-width buttons
// leave, at any terminal width. There is deliberately no "up one level"
// button: the ".." row already at the top of every listing does that, via
// Enter or a double-click, exactly like every other entry. It returns the
// row and the path button separately so the caller can keep updating the
// button's text as fp navigates.
func newNavRow(app *Graphite.Application, fp *filepane.FilePane) (*Graphite.Flex, *Graphite.Button) {
	pathBtn := newPathButton(app, fp)

	row := Graphite.NewFlex(0, 0, 0, 1, Graphite.FlexRow)
	row.Gap = 1
	row.AddChild(newNavButton("<", func() { fp.Back() }), 0)
	row.AddChild(newNavButton(">", func() { fp.Forward() }), 0)
	row.AddChild(newNavButton("Reload", func() { fp.Reload() }), 0)
	row.AddChild(newNavButton("Root", func() { promptChooseRoot(app, fp) }), 0)
	row.AddChild(pathBtn, 1)
	return row, pathBtn
}

// newActionGutter builds the fixed-width column between the two panes
// holding Copy and Move — two buttons, not four: each is a dirButton whose
// arrow flips to always point from the focused pane toward the other one,
// recomputed fresh every frame, rather than a separate static button per
// direction.
func newActionGutter(app *Graphite.Application, left, right *filepane.FilePane) *Graphite.Flex {
	gutter := Graphite.NewFlex(0, 0, 11, 0, Graphite.FlexColumn)
	gutter.Gap = 1
	gutter.AddChild(newDirButton(app, left, right, "Copy", false), 0)
	gutter.AddChild(newDirButton(app, left, right, "Move", true), 0)
	return gutter
}

// dirButton is a Copy/Move gutter button whose label and arrow direction
// reflect whichever pane currently has focus, recomputed every frame
// (there is no "focus changed" event to hook, so drawing fresh is what
// keeps it honest) — a mouse-driven duplicate of F5/F6 for someone who'd
// rather click an explicit direction than rely on "whichever pane is
// active." Deliberately not a Graphite.Button: Button's Text is a plain
// field with no per-frame hook, so a label that must track live state
// needs its own DrawRelative.
type dirButton struct {
	Graphite.BaseWidget
	app         *Graphite.Application
	left, right *filepane.FilePane
	action      string
	move        bool
}

func newDirButton(app *Graphite.Application, left, right *filepane.FilePane, action string, move bool) *dirButton {
	base := Graphite.NewBaseWidget(0, 0, 11, 1)
	return &dirButton{BaseWidget: base, app: app, left: left, right: right, action: action, move: move}
}

// srcDst returns (source, destination) for this click: always from
// whichever pane is focused toward the other one.
func (d *dirButton) srcDst() (src, dst *filepane.FilePane) {
	if d.right.HasFocus() {
		return d.right, d.left
	}
	return d.left, d.right
}

// label returns this frame's button text, arrow pointing toward dst.
func (d *dirButton) label() string {
	if d.right.HasFocus() {
		return "◀ " + d.action
	}
	return "▶ " + d.action
}

// DrawRelative implements Graphite.Widget.
func (d *dirButton) DrawRelative(c *Graphite.Canvas, offX, offY, pW, pH int) {
	d.BaseWidget.DrawRelative(c, offX, offY, pW, pH)
	theme := c.Theme()
	for x := 0; x < d.LastW; x++ {
		c.DrawCell(d.AbsX+x, d.AbsY, " ", theme.BgWidget, theme.FgWindow)
	}
	c.DrawTextBounded(d.AbsX, d.AbsY, d.LastW, "[ "+d.label()+" ]", theme.BgWidget, theme.FgWindow)
}

// HandleEvent implements Graphite.Widget: a click runs Copy/Move with
// srcDst's direction as of this exact click, not whatever it was when the
// button was constructed.
func (d *dirButton) HandleEvent(ev Graphite.Event) {
	if ev.Type != Graphite.EventMouseDown {
		return
	}
	src, dst := d.srcDst()
	doCopyOrMove(d.app, src, dst, d.move)
}

// diskUsageCache is a snapshot of diskspace.Query for one pane, refreshed
// on navigation (see main's refreshDiskUsage) rather than every frame —
// statfs is cheap, but there is no reason to call it on every one of the
// render loop's ~100 iterations per second when the path hasn't changed.
type diskUsageCache struct {
	usage diskspace.Usage
	ok    bool
}

// diskSpaceBar is a full-width row showing a used/total progress bar for
// whichever pane currently has focus. current is read fresh every frame
// (see dirButton for why: there is no "focus changed" hook to update from
// instead), but the underlying diskspace.Query result it reports is
// cached by the caller.
type diskSpaceBar struct {
	Graphite.BaseWidget
	current func() diskUsageCache
}

func newDiskSpaceBar(current func() diskUsageCache) *diskSpaceBar {
	return &diskSpaceBar{BaseWidget: Graphite.NewBaseWidget(0, -2, 0, 1), current: current}
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
		c.DrawCell(d.AbsX+x, d.AbsY, " ", theme.BgScreen, theme.FgWindow)
	}

	cache := d.current()
	if !cache.ok || cache.usage.Total == 0 {
		c.DrawTextBounded(d.AbsX, d.AbsY, d.LastW, "Disk: n/a", theme.BgScreen, theme.FgDisabled)
		return
	}

	used := cache.usage.Total - cache.usage.Free
	usedPct := float64(used) / float64(cache.usage.Total) * 100
	label := fmt.Sprintf("Disk: %s free of %s (%.0f%% used)", formatBytes(cache.usage.Free), formatBytes(cache.usage.Total), usedPct)

	barW := d.LastW - len([]rune(label)) - 3
	if barW < 10 {
		c.DrawTextBounded(d.AbsX, d.AbsY, d.LastW, label, theme.BgScreen, theme.FgWindow)
		return
	}

	c.DrawTextBounded(d.AbsX, d.AbsY, len([]rune(label)), label, theme.BgScreen, theme.FgWindow)
	x := d.AbsX + len([]rune(label)) + 1
	c.DrawCell(x, d.AbsY, "[", theme.BgScreen, theme.FgWindow)
	x++
	filled := int(usedPct / 100 * float64(barW))
	barColor := theme.Primary
	if usedPct >= 90 {
		barColor = theme.Danger
	} else if usedPct >= 75 {
		barColor = theme.Warning
	}
	for i := 0; i < barW; i++ {
		if i < filled {
			c.DrawCell(x+i, d.AbsY, "█", theme.BgScreen, barColor)
		} else {
			c.DrawCell(x+i, d.AbsY, "░", theme.BgScreen, theme.Disabled)
		}
	}
	c.DrawCell(x+barW, d.AbsY, "]", theme.BgScreen, theme.FgWindow)
}

// newFKeyBar builds the bottom action bar. F1/F3/F4/F9 are listed with no
// OnClick (fkeybar renders those dimmed and inert) since they aren't
// implemented yet: F3/F4 need Suspend/Resume (a later phase), and F1/F9
// weren't required by this phase's scope. Showing them dimmed is honest
// about that instead of quietly leaving them off the bar's layout.
func newFKeyBar(app *Graphite.Application, active func() *filepane.FilePane, other func(*filepane.FilePane) *filepane.FilePane) *fkeybar.Bar {
	return fkeybar.New(0, -1, []fkeybar.Key{
		{Label: "F1", Text: "Help"},
		{Label: "F2", Text: "Rename", Role: fkeybar.RolePrimary, OnClick: func() { doRename(app, active()) }},
		{Label: "F3", Text: "View"},
		{Label: "F4", Text: "Edit"},
		{Label: "F5", Text: "Copy", Role: fkeybar.RolePrimary, OnClick: func() { doCopyOrMove(app, active(), other(active()), false) }},
		{Label: "F6", Text: "Move", Role: fkeybar.RoleAccent, OnClick: func() { doCopyOrMove(app, active(), other(active()), true) }},
		{Label: "F7", Text: "MkDir", Role: fkeybar.RoleSuccess, OnClick: func() { doMkdir(app, active()) }},
		{Label: "F8", Text: "Delete", Role: fkeybar.RoleDanger, OnClick: func() { doDelete(app, active()) }},
		{Label: "F9", Text: "Menu"},
		{Label: "F10", Text: "Quit", Role: fkeybar.RoleWarning, OnClick: func() { requestQuit(app) }},
	})
}

// newMenuStrip builds the top menu bar as a mouse-only duplicate of the
// F-key actions and a few operations that otherwise only have a keyboard
// or header-click path (per the project's spec, an optional pointer-driven
// alternative, not a replacement for either). Every item here maps to a
// real, already-implemented action — no category exists just to look like
// a classic commander's fuller menu bar (Total Commander's own Network/
// Configuration categories, say, have no equivalent yet in this phase).
// IsFocusable is forced back to false right after construction, same
// reasoning as newNavButton: MenuStrip only ever responds to
// EventMouseDown anyway (see graphite's widgets.go), so it loses no
// functionality by staying out of the Tab cycle, and the two FilePanes
// stay the only two top-level focusable widgets in the window.
func newMenuStrip(app *Graphite.Application, active func() *filepane.FilePane, other func(*filepane.FilePane) *filepane.FilePane) *Graphite.MenuStrip {
	menu := Graphite.NewMenuStrip([]Graphite.MenuCategory{
		{Label: "File", Items: []Graphite.MenuItem{
			{Label: "Rename        F2", Action: func() { doRename(app, active()) }},
			{Label: "Copy          F5", Action: func() { doCopyOrMove(app, active(), other(active()), false) }},
			{Label: "Move          F6", Action: func() { doCopyOrMove(app, active(), other(active()), true) }},
			{Label: "New Folder    F7", Action: func() { doMkdir(app, active()) }},
			{Label: "Delete        F8", Action: func() { doDelete(app, active()) }},
			{Label: "Quit         F10", Action: func() { requestQuit(app) }},
		}},
		{Label: "Mark", Items: []Graphite.MenuItem{
			{Label: "Tag/Untag    Ins", Action: func() { active().ToggleTag() }},
		}},
		{Label: "View", Items: []Graphite.MenuItem{
			{Label: "Sort by Name", Action: func() { active().SetSort(filepane.SortByName) }},
			{Label: "Sort by Size", Action: func() { active().SetSort(filepane.SortBySize) }},
			{Label: "Sort by Date", Action: func() { active().SetSort(filepane.SortByDate) }},
			{Label: "Refresh", Action: func() { active().Reload() }},
		}},
		{Label: "Help", Items: []Graphite.MenuItem{
			{Label: "About", Action: func() {
				app.ShowMessage(" About ", "Diskette — a dual-pane file manager.", Graphite.BtnDefault)
			}},
		}},
	})
	menu.IsFocusable = false
	return menu
}

// requestQuit asks the user to confirm before actually quitting — wired to
// both Escape (via Application.SetOnQuitRequested) and F10.
func requestQuit(app *Graphite.Application) {
	Graphite.ShowConfirm(app, " Quit ", "Quit Diskette?", Graphite.BtnDanger, func() {
		app.Quit()
	})
}

// newPathButton creates the clickable bar showing fp's current path.
// Width is forced to 0 so it stretches to fill whatever space newNavRow's
// Flex offers it, rather than staying sized to the path it was
// constructed with.
func newPathButton(app *Graphite.Application, fp *filepane.FilePane) *Graphite.Button {
	btn := Graphite.NewButton(0, 0, fp.Path(), Graphite.BtnDefault, func() {
		promptGoTo(app, fp)
	})
	btn.IsFocusable = false
	btn.Width = 0
	return btn
}

// setButtonText updates a path button's label.
func setButtonText(b *Graphite.Button, text string) {
	b.Text = text
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
