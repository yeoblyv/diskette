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
	"github.com/yeoblyv/diskette/internal/filepane"
	"github.com/yeoblyv/diskette/internal/theme"
	"github.com/yeoblyv/diskette/internal/vfs"
)

const fKeyBarText = " F1 Help  F2 Rename  F3 View  F4 Edit  F5 Copy  F6 Move  F7 MkDir  F8 Delete  F9 Menu  F10 Quit"

func main() {
	start, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "diskette:", err)
		os.Exit(1)
	}

	app := Graphite.NewApplication()
	app.SetTheme(theme.Diskette())
	win := Graphite.NewWindow(0, 0, " diskette ")
	win.SetPercentSize(96, 92)

	fs := vfs.LocalFS{}

	left := filepane.New(0, 2, 0, -1, fs, start)
	left.SetPercentLayout(0, 0, 50, 0)
	right := filepane.New(0, 2, 0, -1, fs, start)
	right.SetPercentLayout(50, 0, 50, 0)

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

	leftPathBtn := newPathButton(app, left)
	leftPathBtn.SetPosition(0, 1)
	rightPathBtn := newPathButton(app, right)
	rightPathBtn.SetPosition(0, 1)
	rightPathBtn.SetPercentLayout(50, 0, 0, 0)
	left.OnPathChanged = func(path string) { setButtonText(leftPathBtn, path) }
	right.OnPathChanged = func(path string) { setButtonText(rightPathBtn, path) }

	fKeyBar := Graphite.NewLabel(0, -1, fKeyBarText)

	menu := newMenuStrip(app, activePane, otherPane)

	win.AddWidget(menu)
	win.AddWidget(leftPathBtn)
	win.AddWidget(rightPathBtn)
	win.AddWidget(left)
	win.AddWidget(right)
	win.AddWidget(fKeyBar)

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

// newMenuStrip builds the top menu bar as a mouse-only duplicate of the
// F-key actions (per the project's spec, an optional pointer-driven
// alternative to the F-key bar, not a replacement for it). IsFocusable is
// forced back to false right after construction, same reasoning as
// newPathButton: MenuStrip only ever responds to EventMouseDown anyway
// (see graphite's widgets.go), so it loses no functionality by staying out
// of the Tab cycle, and the two FilePanes stay the only two top-level
// focusable widgets in the window.
func newMenuStrip(app *Graphite.Application, active func() *filepane.FilePane, other func(*filepane.FilePane) *filepane.FilePane) *Graphite.MenuStrip {
	menu := Graphite.NewMenuStrip([]Graphite.MenuCategory{
		{Label: "File", Items: []Graphite.MenuItem{
			{Label: "Rename  F2", Action: func() { doRename(app, active()) }},
			{Label: "Copy    F5", Action: func() { doCopyOrMove(app, active(), other(active()), false) }},
			{Label: "Move    F6", Action: func() { doCopyOrMove(app, active(), other(active()), true) }},
			{Label: "MkDir   F7", Action: func() { doMkdir(app, active()) }},
			{Label: "Delete  F8", Action: func() { doDelete(app, active()) }},
		}},
		{Label: "Help", Items: []Graphite.MenuItem{
			{Label: "About", Action: func() {
				app.ShowMessage(" About ", "Diskette — a dual-pane file manager.", Graphite.BtnDefault)
			}},
		}},
		{Label: "Quit", Items: []Graphite.MenuItem{
			{Label: "Quit    F10", Action: func() { requestQuit(app) }},
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

// newPathButton creates the clickable bar showing fp's current path. It is
// deliberately not focusable (IsFocusable is forced back to false right
// after construction): the two FilePanes must stay the only two top-level
// focusable widgets in the window, so Tab keeps switching directly between
// them — see the project's architecture notes on this. Clicking it still
// works regardless of focusability, since Window delivers a mouse click to
// whatever it hit-tests under the pointer independently of the Tab order.
func newPathButton(app *Graphite.Application, fp *filepane.FilePane) *Graphite.Button {
	btn := Graphite.NewButton(0, 0, fp.Path(), Graphite.BtnDefault, func() {
		promptGoTo(app, fp)
	})
	btn.IsFocusable = false
	return btn
}

// setButtonText updates a path button's label and resizes it to fit,
// mirroring what Label.SetText does for a Button.
func setButtonText(b *Graphite.Button, text string) {
	b.Text = text
	b.Width = len([]rune(text)) + 4
}

// promptGoTo is Diskette's cd-bar: a modal prompting for a path, rather
// than a persistent input row, precisely so it never becomes a third
// top-level focusable widget (see newPathButton).
func promptGoTo(app *Graphite.Application, fp *filepane.FilePane) {
	Graphite.ShowTextEditor(app, " Go to folder ", "Path:", fp.Path(), func(path string) {
		if _, err := fp.FS.Stat(context.Background(), path); err != nil {
			app.ShowMessage(" Error ", err.Error(), Graphite.BtnDanger)
			return
		}
		fp.SetPath(path)
	})
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
	mod.AddWidget(Graphite.NewButton(2, 6, "Cancel", Graphite.BtnDefault, func() {
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
