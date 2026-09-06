// Package filepane implements FilePane, the commander-style directory pane
// Diskette's two panels are built from. It is a custom Graphite widget
// rather than a Graphite.ListBox, since a commander pane needs
// multi-selection ("tagging"), sortable Name/Size/Date/Attr columns, and
// directory navigation — none of which ListBox's single-selection model
// supports.
package filepane

import (
	"context"
	"fmt"
	"strings"

	Graphite "github.com/yeoblyv/graphite"

	"github.com/yeoblyv/diskette/internal/vfs"
)

// SortField selects which column FilePane orders its listing by.
type SortField int

// Supported sort fields. Directories always sort before files regardless
// of field, matching every classic dual-pane commander.
const (
	SortByName SortField = iota
	SortBySize
	SortByDate
)

// row is one visible line: either a real filesystem entry or the synthetic
// ".." row used to navigate to the parent directory.
type row struct {
	vfs.Entry
	isParent bool
	tagged   bool
}

// FilePane is a focusable, scrollable directory listing with Name/Size/
// Date/Attr columns, tagging (multi-selection), and column sorting.
type FilePane struct {
	Graphite.BaseWidget

	FS   vfs.FileSystem
	path string
	rows []row

	cursor int
	scroll int

	sortField SortField
	sortDesc  bool

	// OnPathChanged, if set, is called after every successful navigation
	// (SetPath or Enter on a directory row) with the pane's new path —
	// the host program uses it to keep a window title or status line in
	// sync.
	OnPathChanged func(path string)

	// OnFunctionKey, if set, is called for every F1-F12 keypress this
	// pane's HandleEvent receives. FilePane has no opinion on what any
	// function key does — the host program wires a shared F-key bar
	// across both panes and reads HasFocus() (or its own bookkeeping) to
	// tell which pane an action should apply to.
	OnFunctionKey func(key Graphite.KeyCode)
}

// New creates a FilePane at (x, y, w, h) — 0/negative w or h stretch to
// fill the remaining parent space, per Graphite.BaseWidget's convention —
// browsing fs starting at path.
func New(x, y, w, h int, fs vfs.FileSystem, path string) *FilePane {
	base := Graphite.NewBaseWidget(x, y, w, h)
	base.IsFocusable = true
	fp := &FilePane{BaseWidget: base, FS: fs}
	fp.SetPath(path)
	return fp
}

// Path returns the directory currently listed.
func (fp *FilePane) Path() string { return fp.path }

// Selected returns the entry under the cursor, or false if the cursor is
// on the ".." row or the listing is empty.
func (fp *FilePane) Selected() (vfs.Entry, bool) {
	if fp.cursor < 0 || fp.cursor >= len(fp.rows) || fp.rows[fp.cursor].isParent {
		return vfs.Entry{}, false
	}
	return fp.rows[fp.cursor].Entry, true
}

// SelectionPaths returns the absolute paths of every tagged row, or — if
// nothing is tagged — the single entry under the cursor, matching the
// classic commander convention that an operation with nothing explicitly
// tagged acts on whatever the cursor is sitting on. Returns nil if the
// cursor is on ".." and nothing is tagged.
func (fp *FilePane) SelectionPaths() []string {
	var out []string
	for _, r := range fp.rows {
		if r.tagged {
			out = append(out, fp.FS.Join(fp.path, r.Name))
		}
	}
	if len(out) > 0 {
		return out
	}
	if e, ok := fp.Selected(); ok {
		return []string{fp.FS.Join(fp.path, e.Name)}
	}
	return nil
}

// SetPath lists dir and navigates there, resetting the cursor, scroll
// position, and any tags. It leaves the pane on its current path if dir
// cannot be listed (e.g. permission denied).
func (fp *FilePane) SetPath(dir string) {
	entries, err := fp.FS.List(context.Background(), dir)
	if err != nil {
		return
	}

	fp.path = dir
	fp.rows = fp.rows[:0]
	if _, ok := fp.FS.Parent(dir); ok {
		fp.rows = append(fp.rows, row{isParent: true})
	}
	for _, e := range entries {
		fp.rows = append(fp.rows, row{Entry: e})
	}
	fp.sortRows()

	fp.cursor = 0
	fp.scroll = 0

	if fp.OnPathChanged != nil {
		fp.OnPathChanged(fp.path)
	}
}

// Reload re-lists the current directory, preserving the cursor position
// and tags by entry name where those entries still exist. Callers refresh
// a pane with this after an operation (copy/move/delete/mkdir/rename) that
// changed its contents from outside FilePane itself.
func (fp *FilePane) Reload() {
	var cursorName string
	if fp.cursor >= 0 && fp.cursor < len(fp.rows) {
		cursorName = fp.rows[fp.cursor].Name
	}
	tagged := make(map[string]bool)
	for _, r := range fp.rows {
		if r.tagged {
			tagged[r.Name] = true
		}
	}

	entries, err := fp.FS.List(context.Background(), fp.path)
	if err != nil {
		return
	}

	fp.rows = fp.rows[:0]
	if _, ok := fp.FS.Parent(fp.path); ok {
		fp.rows = append(fp.rows, row{isParent: true})
	}
	for _, e := range entries {
		fp.rows = append(fp.rows, row{Entry: e, tagged: tagged[e.Name]})
	}
	fp.sortRows()

	fp.cursor = 0
	for i, r := range fp.rows {
		if r.Name == cursorName {
			fp.cursor = i
			break
		}
	}
	fp.clampScroll()
}

// activateCursor navigates into the directory (or ".." parent) under the
// cursor. It does nothing for a file row — opening/viewing/editing a file
// is the host program's concern (F3/F4), not FilePane's.
func (fp *FilePane) activateCursor() {
	if fp.cursor < 0 || fp.cursor >= len(fp.rows) {
		return
	}
	r := fp.rows[fp.cursor]
	if r.isParent {
		if parent, ok := fp.FS.Parent(fp.path); ok {
			fp.SetPath(parent)
		}
		return
	}
	if r.IsDir {
		fp.SetPath(fp.FS.Join(fp.path, r.Name))
	}
}

// toggleTagAndAdvance flips the tag on the cursor row and moves the
// cursor down one — Insert's classic commander behavior. It is a no-op on
// the ".." row, which can never be tagged.
func (fp *FilePane) toggleTagAndAdvance() {
	if fp.cursor < 0 || fp.cursor >= len(fp.rows) || fp.rows[fp.cursor].isParent {
		return
	}
	fp.rows[fp.cursor].tagged = !fp.rows[fp.cursor].tagged
	if fp.cursor < len(fp.rows)-1 {
		fp.cursor++
		fp.clampScroll()
	}
}

// setSort changes the sort field, toggling direction if field is already
// the active one (matching a commander's usual double-click-header
// behavior), and re-sorts in place.
func (fp *FilePane) setSort(field SortField) {
	if fp.sortField == field {
		fp.sortDesc = !fp.sortDesc
	} else {
		fp.sortField = field
		fp.sortDesc = false
	}
	fp.sortRows()
}

// sortRows orders rows by the active field/direction, always pinning ".."
// first and directories before files.
func (fp *FilePane) sortRows() {
	less := func(i, j int) bool {
		a, b := fp.rows[i], fp.rows[j]
		if a.isParent || b.isParent {
			return a.isParent && !b.isParent
		}
		if a.IsDir != b.IsDir {
			return a.IsDir
		}

		cmp := 0
		switch fp.sortField {
		case SortBySize:
			switch {
			case a.Size < b.Size:
				cmp = -1
			case a.Size > b.Size:
				cmp = 1
			}
		case SortByDate:
			switch {
			case a.ModTime.Before(b.ModTime):
				cmp = -1
			case a.ModTime.After(b.ModTime):
				cmp = 1
			}
		default:
			cmp = strings.Compare(a.Name, b.Name)
		}
		if cmp == 0 {
			cmp = strings.Compare(a.Name, b.Name)
		}
		if fp.sortDesc {
			cmp = -cmp
		}
		return cmp < 0
	}
	insertionSort(fp.rows, less)
}

// insertionSort is a small stable sort over rows, avoiding a dependency on
// sort.Slice's reflection-based closure for a slice this short (a
// directory listing rarely exceeds a few thousand entries, where the
// difference is immaterial either way — this exists so sortRows reads as
// plain, allocation-free code).
func insertionSort(rows []row, less func(i, j int) bool) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && less(j, j-1); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

// moveCursor shifts the cursor by delta rows, clamped to the listing
// bounds, adjusting scroll to keep it visible.
func (fp *FilePane) moveCursor(delta int) {
	if len(fp.rows) == 0 {
		return
	}
	fp.cursor += delta
	if fp.cursor < 0 {
		fp.cursor = 0
	}
	if fp.cursor >= len(fp.rows) {
		fp.cursor = len(fp.rows) - 1
	}
	fp.clampScroll()
}

// clampScroll adjusts scroll so the cursor row stays within the visible
// listing area (the widget's height minus the one header row).
func (fp *FilePane) clampScroll() {
	visible := fp.LastH - 1
	if visible < 1 {
		visible = 1
	}
	if fp.cursor < fp.scroll {
		fp.scroll = fp.cursor
	}
	if fp.cursor >= fp.scroll+visible {
		fp.scroll = fp.cursor - visible + 1
	}
	if fp.scroll < 0 {
		fp.scroll = 0
	}
}

// HandleEvent implements Graphite.Widget.
func (fp *FilePane) HandleEvent(ev Graphite.Event) {
	switch ev.Type {
	case Graphite.EventKey:
		switch ev.Key {
		case Graphite.KeyUp:
			fp.moveCursor(-1)
		case Graphite.KeyDown:
			fp.moveCursor(1)
		case Graphite.KeyEnter:
			fp.activateCursor()
		case Graphite.KeyInsert:
			fp.toggleTagAndAdvance()
		default:
			if isFunctionKey(ev.Key) && fp.OnFunctionKey != nil {
				fp.OnFunctionKey(ev.Key)
			}
		}

	case Graphite.EventMouseDown:
		relY := ev.MouseY - fp.AbsY
		if relY == 0 {
			fp.handleHeaderClick(ev.MouseX - fp.AbsX)
			return
		}
		idx := fp.scroll + relY - 1
		if idx >= 0 && idx < len(fp.rows) {
			fp.cursor = idx
			fp.clampScroll()
		}

	case Graphite.EventMouseScrollUp:
		if fp.scroll > 0 {
			fp.scroll--
		}
	case Graphite.EventMouseScrollDown:
		maxScroll := len(fp.rows) - (fp.LastH - 1)
		if maxScroll > 0 && fp.scroll < maxScroll {
			fp.scroll++
		}
	}
}

// isFunctionKey reports whether key is one of F1-F12.
func isFunctionKey(key Graphite.KeyCode) bool {
	switch key {
	case Graphite.KeyF1, Graphite.KeyF2, Graphite.KeyF3, Graphite.KeyF4,
		Graphite.KeyF5, Graphite.KeyF6, Graphite.KeyF7, Graphite.KeyF8,
		Graphite.KeyF9, Graphite.KeyF10, Graphite.KeyF11, Graphite.KeyF12:
		return true
	default:
		return false
	}
}

// Fixed widths, in characters, of the optional columns. Name always gets
// whatever space is left over.
const (
	sizeColWidth = 8
	dateColWidth = 14 // "02.01.06 15:04"
	attrColWidth = 10 // e.g. "-rw-r--r--"

	// minNameWidth is the least a Name column can shrink to before an
	// optional column is dropped instead — two panes side by side in an
	// 80-column terminal leave roughly 35 usable columns each, nowhere
	// near enough for Name plus all three fixed columns (33 chars on
	// their own), so a narrow pane must degrade by hiding columns rather
	// than leaving Name too thin to read any name at all.
	minNameWidth = 10
)

// colLayout is where each visible column starts, relative to the pane's
// own AbsX, and how wide Name is. A hidden optional column's x is -1.
type colLayout struct {
	nameW int
	sizeX int
	dateX int
	attrX int
}

// layout picks the widest column set that still leaves Name at least
// minNameWidth wide, dropping Attr first and then Date as the pane
// narrows — the same graceful degradation a classic commander applies
// rather than truncating every filename to nothing.
func (fp *FilePane) layout() colLayout {
	w := fp.LastW

	if rem := w - (1 + sizeColWidth + 1 + dateColWidth + 1 + attrColWidth); rem >= minNameWidth {
		sizeX := rem + 1
		dateX := sizeX + sizeColWidth + 1
		attrX := dateX + dateColWidth + 1
		return colLayout{nameW: rem, sizeX: sizeX, dateX: dateX, attrX: attrX}
	}
	if rem := w - (1 + sizeColWidth + 1 + dateColWidth); rem >= minNameWidth {
		sizeX := rem + 1
		dateX := sizeX + sizeColWidth + 1
		return colLayout{nameW: rem, sizeX: sizeX, dateX: dateX, attrX: -1}
	}
	if rem := w - (1 + sizeColWidth); rem >= minNameWidth {
		return colLayout{nameW: rem, sizeX: rem + 1, dateX: -1, attrX: -1}
	}

	nameW := w
	if nameW < 1 {
		nameW = 1
	}
	return colLayout{nameW: nameW, sizeX: -1, dateX: -1, attrX: -1}
}

// handleHeaderClick maps a click's x offset within the header row to the
// column it landed on and toggles sorting by that column. Clicking where a
// hidden column would be (or past the last visible one) does nothing.
func (fp *FilePane) handleHeaderClick(relX int) {
	l := fp.layout()
	switch {
	case relX < l.nameW:
		fp.setSort(SortByName)
	case l.sizeX >= 0 && relX < l.sizeX+sizeColWidth:
		fp.setSort(SortBySize)
	case l.dateX >= 0 && relX < l.dateX+dateColWidth:
		fp.setSort(SortByDate)
	}
}

// formatSize renders a byte count the way commanders traditionally do:
// exact bytes below 1000, otherwise one decimal in the largest unit that
// keeps the number under 1000.
func formatSize(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for f := n / unit; f >= unit; f /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// DrawRelative implements Graphite.Widget.
func (fp *FilePane) DrawRelative(c *Graphite.Canvas, offX, offY, pW, pH int) {
	fp.BaseWidget.DrawRelative(c, offX, offY, pW, pH)
	fp.clampScroll()

	headerBg := c.Theme().BgWidget
	fgDim := c.Theme().FgDisabled
	l := fp.layout()

	for i := 0; i < fp.LastW; i++ {
		c.DrawCell(fp.AbsX+i, fp.AbsY, " ", headerBg, fgDim)
	}
	c.DrawTextBounded(fp.AbsX, fp.AbsY, l.nameW, sortLabel("Name", fp.sortField == SortByName, fp.sortDesc), headerBg, fgDim)
	if l.sizeX >= 0 {
		c.DrawTextBounded(fp.AbsX+l.sizeX, fp.AbsY, sizeColWidth, sortLabel("Size", fp.sortField == SortBySize, fp.sortDesc), headerBg, fgDim)
	}
	if l.dateX >= 0 {
		c.DrawTextBounded(fp.AbsX+l.dateX, fp.AbsY, dateColWidth, sortLabel("Date", fp.sortField == SortByDate, fp.sortDesc), headerBg, fgDim)
	}
	if l.attrX >= 0 {
		c.DrawTextBounded(fp.AbsX+l.attrX, fp.AbsY, attrColWidth, "Attr", headerBg, fgDim)
	}

	visible := fp.LastH - 1
	for i := 0; i < visible; i++ {
		idx := fp.scroll + i
		y := fp.AbsY + 1 + i
		if idx >= len(fp.rows) {
			for x := 0; x < fp.LastW; x++ {
				c.DrawCell(fp.AbsX+x, y, " ", c.Theme().BgWidget, c.Theme().FgWindow)
			}
			continue
		}
		fp.drawRow(c, y, l, fp.rows[idx], idx == fp.cursor)
	}
}

// drawRow renders one listing row at absolute row y.
func (fp *FilePane) drawRow(c *Graphite.Canvas, y int, l colLayout, r row, isCursor bool) {
	bg, fg := c.Theme().BgWidget, c.Theme().FgWindow
	switch {
	case isCursor && fp.IsFocused:
		bg, fg = c.Theme().Primary, c.Theme().FgFocused
	case isCursor:
		bg = c.Theme().BgFocused.Darken(0.6)
	case r.tagged:
		fg = c.Theme().Accent
	case r.IsDir:
		fg = c.Theme().Success
	}

	for x := 0; x < fp.LastW; x++ {
		c.DrawCell(fp.AbsX+x, y, " ", bg, fg)
	}

	name := r.Name
	if r.isParent {
		name = ".."
	} else if r.IsDir {
		name = "/" + name
	}
	marker := "  "
	if r.tagged {
		marker = "● "
	}
	c.DrawTextBounded(fp.AbsX, y, l.nameW, marker+name, bg, fg)

	if r.isParent {
		return
	}
	if l.sizeX >= 0 {
		sizeStr := ""
		if !r.IsDir {
			sizeStr = formatSize(r.Size)
		}
		c.DrawTextBounded(fp.AbsX+l.sizeX, y, sizeColWidth, padLeft(sizeStr, sizeColWidth), bg, fg)
	}
	if l.dateX >= 0 {
		c.DrawTextBounded(fp.AbsX+l.dateX, y, dateColWidth, r.ModTime.Format("02.01.06 15:04"), bg, fg)
	}
	if l.attrX >= 0 {
		c.DrawTextBounded(fp.AbsX+l.attrX, y, attrColWidth, r.Mode, bg, fg)
	}
}

// padLeft right-aligns s within width w by prepending spaces, leaving s
// unchanged if it is already at least w runes wide.
func padLeft(s string, w int) string {
	n := w - len([]rune(s))
	if n <= 0 {
		return s
	}
	return strings.Repeat(" ", n) + s
}

// sortLabel appends a small arrow to label when it names the active sort
// field, so the header shows which column and direction is applied.
func sortLabel(label string, active, desc bool) string {
	if !active {
		return label
	}
	if desc {
		return label + " ▼"
	}
	return label + " ▲"
}
