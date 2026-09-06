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
	"math"
	"strings"
	"time"
	"unicode"

	Graphite "github.com/yeoblyv/graphite"

	"github.com/yeoblyv/diskette/internal/vfs"
)

// doubleClickWindow is how close together two clicks on the same row must
// land to count as a double-click, matching graphite's own ListBox/Fader
// convention.
const doubleClickWindow = 500 * time.Millisecond

// searchTimeout is how long a pause between keystrokes resets the
// quick-search buffer, so typing a fresh word doesn't append to a stale one.
const searchTimeout = time.Second

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

	backStack    []string
	forwardStack []string

	lastClickIdx  int
	lastClickTime time.Time

	searchBuf    string
	lastSearchAt time.Time

	// OnPathChanged, if set, is called after every successful navigation
	// (SetPath, Back/Forward/Up, or Enter on a directory row) with the
	// pane's new path — the host program uses it to keep a window title
	// or status line in sync.
	OnPathChanged func(path string)

	// OnFunctionKey, if set, is called for every F1-F12 keypress this
	// pane's HandleEvent receives. FilePane has no opinion on what any
	// function key does — the host program wires a shared F-key bar
	// across both panes and reads HasFocus() (or its own bookkeeping) to
	// tell which pane an action should apply to.
	OnFunctionKey func(key Graphite.KeyCode)

	// OnSearchChanged, if set, is called every time typeAhead's
	// quick-search buffer changes, so the host program can surface it
	// (e.g. in a status line) — FilePane itself only draws the jumped-to
	// selection, not the query text.
	OnSearchChanged func(query string)

	// OnOpenFile, if set, is called with a row's full path when Enter or a
	// double-click activates a regular file (not a directory or the ".."
	// row) — the host program's hook for launching it in the OS's
	// associated default application, distinct from F3/F4's $PAGER/$EDITOR.
	OnOpenFile func(path string)
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
// cannot be listed (e.g. permission denied). This is a fresh navigation:
// it pushes the pane's current path onto the back-navigation stack and
// clears the forward stack, exactly like following a link in a browser.
func (fp *FilePane) SetPath(dir string) {
	fp.navigate(dir, true)
}

// navigate is the shared implementation behind SetPath, Back, and Forward.
// addToHistory controls whether the pane's current path is pushed onto the
// back stack first — Back/Forward manage the stacks themselves and pass
// false so stepping through history doesn't also grow it. It reports
// whether dir could actually be listed.
func (fp *FilePane) navigate(dir string, addToHistory bool) bool {
	entries, err := fp.FS.List(context.Background(), dir)
	if err != nil {
		return false
	}

	if addToHistory && fp.path != "" && fp.path != dir {
		fp.backStack = append(fp.backStack, fp.path)
		fp.forwardStack = fp.forwardStack[:0]
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
	return true
}

// Up navigates to the parent directory (equivalent to activating the ".."
// row), leaving the cursor on the entry that was just left — so stepping
// in and immediately back out lands exactly where you started, instead of
// resetting to the top of the parent's listing.
func (fp *FilePane) Up() {
	parent, ok := fp.FS.Parent(fp.path)
	if !ok {
		return
	}
	childName := fp.baseName()
	if fp.navigate(parent, true) {
		fp.selectByName(childName)
	}
}

// baseName returns the current directory's own name — its last path
// segment — by stripping its parent's path from fp.path. FileSystem has
// no dedicated "base name" method; Parent already gives us the exact
// prefix to strip, and TrimLeft handles both POSIX and Windows separators
// without needing to know which convention this FS uses.
func (fp *FilePane) baseName() string {
	parent, ok := fp.FS.Parent(fp.path)
	if !ok {
		return fp.path
	}
	return strings.TrimLeft(strings.TrimPrefix(fp.path, parent), `/\`)
}

// selectByName moves the cursor to the row named name, if present, without
// changing the current directory or scrolling further than necessary.
func (fp *FilePane) selectByName(name string) {
	for i, r := range fp.rows {
		if r.Name == name {
			fp.cursor = i
			fp.clampScroll()
			return
		}
	}
}

// Back navigates to the previously visited path, if any, pushing the
// current path onto the forward stack so Forward can return to it.
func (fp *FilePane) Back() {
	if len(fp.backStack) == 0 {
		return
	}
	prev := fp.backStack[len(fp.backStack)-1]
	leaving := fp.path
	if !fp.navigate(prev, false) {
		return
	}
	fp.backStack = fp.backStack[:len(fp.backStack)-1]
	fp.forwardStack = append(fp.forwardStack, leaving)
}

// Forward navigates to the path Back most recently left, if any.
func (fp *FilePane) Forward() {
	if len(fp.forwardStack) == 0 {
		return
	}
	next := fp.forwardStack[len(fp.forwardStack)-1]
	leaving := fp.path
	if !fp.navigate(next, false) {
		return
	}
	fp.forwardStack = fp.forwardStack[:len(fp.forwardStack)-1]
	fp.backStack = append(fp.backStack, leaving)
}

// CanGoBack reports whether Back would navigate anywhere.
func (fp *FilePane) CanGoBack() bool { return len(fp.backStack) > 0 }

// CanGoForward reports whether Forward would navigate anywhere.
func (fp *FilePane) CanGoForward() bool { return len(fp.forwardStack) > 0 }

// ToggleTag flips the tag on the cursor row and advances the cursor,
// exactly like pressing Insert — exported so a host program's own UI (a
// menu item, say) can trigger the same tagging gesture Insert does.
func (fp *FilePane) ToggleTag() { fp.toggleTagAndAdvance() }

// SetSort changes the sort field, toggling direction if field is already
// active, and re-sorts in place — exported so a host program's own UI (a
// menu item, say) can trigger the same sort change clicking a column
// header does.
func (fp *FilePane) SetSort(field SortField) { fp.setSort(field) }

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
// cursor. For a file row it calls OnOpenFile with the file's full path, if
// set — opening it is the host program's concern, not FilePane's.
func (fp *FilePane) activateCursor() {
	if fp.cursor < 0 || fp.cursor >= len(fp.rows) {
		return
	}
	r := fp.rows[fp.cursor]
	if r.isParent {
		fp.Up()
		return
	}
	if r.IsDir {
		fp.SetPath(fp.FS.Join(fp.path, r.Name))
		return
	}
	if fp.OnOpenFile != nil {
		fp.OnOpenFile(fp.FS.Join(fp.path, r.Name))
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

// visibleRows returns how many rows are available for the listing itself,
// after the one header row and one bottom status row (see statusLine).
func (fp *FilePane) visibleRows() int {
	v := fp.LastH - 2
	if v < 1 {
		v = 1
	}
	return v
}

// clampScroll adjusts scroll so the cursor row stays within the visible
// listing area.
func (fp *FilePane) clampScroll() {
	visible := fp.visibleRows()
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
		case Graphite.KeyNone:
			if ev.CharCode != 0 {
				fp.typeAhead(ev.CharCode)
			}
		default:
			if isFunctionKey(ev.Key) && fp.OnFunctionKey != nil {
				fp.OnFunctionKey(ev.Key)
			}
		}

	case Graphite.EventMouseDown, Graphite.EventMouseDrag:
		relY := ev.MouseY - fp.AbsY
		if ev.Type == Graphite.EventMouseDown && relY == 0 {
			fp.handleHeaderClick(ev.MouseX - fp.AbsX)
			return
		}
		if ev.MouseX == fp.AbsX+fp.LastW-1 && len(fp.rows) > fp.visibleRows() {
			fp.scrollToClick(relY - 1)
			return
		}
		if ev.Type == Graphite.EventMouseDrag {
			return
		}
		idx := fp.scroll + relY - 1
		if idx >= 0 && idx < len(fp.rows) {
			fp.cursor = idx
			fp.clampScroll()

			now := time.Now()
			if fp.lastClickIdx == idx && now.Sub(fp.lastClickTime) < doubleClickWindow {
				fp.activateCursor()
			}
			fp.lastClickIdx = idx
			fp.lastClickTime = now
		}

	case Graphite.EventMouseScrollUp:
		if fp.scroll > 0 {
			fp.scroll--
		}
	case Graphite.EventMouseScrollDown:
		if maxScroll := fp.scrollbarMaxScroll(); fp.scroll < maxScroll {
			fp.scroll++
		}
	}
}

// typeAhead implements quick-search: typing accumulates a query (reset
// after searchTimeout of no typing) and jumps the cursor to the first
// non-".." row whose name starts with it, case-insensitively — the
// classic commander "just start typing to find a file" gesture.
func (fp *FilePane) typeAhead(r rune) {
	now := time.Now()
	if fp.lastSearchAt.IsZero() || now.Sub(fp.lastSearchAt) > searchTimeout {
		fp.searchBuf = ""
	}
	fp.searchBuf += string(unicode.ToLower(r))
	fp.lastSearchAt = now

	for i, row := range fp.rows {
		if row.isParent {
			continue
		}
		if strings.HasPrefix(strings.ToLower(row.Name), fp.searchBuf) {
			fp.cursor = i
			fp.clampScroll()
			break
		}
	}

	if fp.OnSearchChanged != nil {
		fp.OnSearchChanged(fp.searchBuf)
	}
}

// SearchQuery returns the in-progress quick-search text (see typeAhead),
// or "" once searchTimeout has passed since the last keystroke.
func (fp *FilePane) SearchQuery() string {
	if fp.lastSearchAt.IsZero() || time.Since(fp.lastSearchAt) > searchTimeout {
		return ""
	}
	return fp.searchBuf
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
	extColWidth  = 5 // e.g. "html"
	sizeColWidth = 8
	dateColWidth = 14 // "02.01.06 15:04"
	attrColWidth = 10 // e.g. "-rw-r--r--"

	// minNameWidth is the least a Name column can shrink to before an
	// optional column is dropped instead — two panes side by side in an
	// 80-column terminal leave roughly 35 usable columns each, nowhere
	// near enough for Name plus all four fixed columns, so a narrow pane
	// must degrade by hiding columns rather than leaving Name too thin to
	// read any name at all.
	minNameWidth = 10
)

// colLayout is where each visible column starts, relative to the pane's
// own AbsX, and how wide Name is. A hidden optional column's x is -1.
type colLayout struct {
	nameW int
	extX  int
	sizeX int
	dateX int
	attrX int
}

// layout picks the widest column set that still leaves Name at least
// minNameWidth wide, dropping Attr first, then Date, then Ext as the pane
// narrows — the same graceful degradation a classic commander applies
// rather than truncating every filename to nothing.
func (fp *FilePane) layout() colLayout {
	w := fp.LastW

	if rem := w - (1 + extColWidth + 1 + sizeColWidth + 1 + dateColWidth + 1 + attrColWidth); rem >= minNameWidth {
		extX := rem + 1
		sizeX := extX + extColWidth + 1
		dateX := sizeX + sizeColWidth + 1
		attrX := dateX + dateColWidth + 1
		return colLayout{nameW: rem, extX: extX, sizeX: sizeX, dateX: dateX, attrX: attrX}
	}
	if rem := w - (1 + extColWidth + 1 + sizeColWidth + 1 + dateColWidth); rem >= minNameWidth {
		extX := rem + 1
		sizeX := extX + extColWidth + 1
		dateX := sizeX + sizeColWidth + 1
		return colLayout{nameW: rem, extX: extX, sizeX: sizeX, dateX: dateX, attrX: -1}
	}
	if rem := w - (1 + sizeColWidth + 1 + dateColWidth); rem >= minNameWidth {
		sizeX := rem + 1
		dateX := sizeX + sizeColWidth + 1
		return colLayout{nameW: rem, extX: -1, sizeX: sizeX, dateX: dateX, attrX: -1}
	}
	if rem := w - (1 + sizeColWidth); rem >= minNameWidth {
		return colLayout{nameW: rem, extX: -1, sizeX: rem + 1, dateX: -1, attrX: -1}
	}

	nameW := w
	if nameW < 1 {
		nameW = 1
	}
	return colLayout{nameW: nameW, extX: -1, sizeX: -1, dateX: -1, attrX: -1}
}

// splitExt separates a file's base name from its extension (without the
// dot), for the classic commander Name/Ext column split. A name with no
// dot, or a dotfile like ".gitignore" whose only dot is its first
// character, has no extension.
func splitExt(name string) (base, ext string) {
	i := strings.LastIndex(name, ".")
	if i <= 0 {
		return name, ""
	}
	return name[:i], name[i+1:]
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

	theme := c.Theme()
	headerBg := theme.BgWidget
	if fp.IsFocused {
		// A tinted header, not just the cursor row, so which pane is
		// active reads at a glance regardless of scroll position —
		// two panes with an otherwise identical dark theme are hard to
		// tell apart by the cursor highlight alone.
		headerBg = theme.Primary.Darken(0.75)
	}
	fgDim := theme.FgDisabled
	l := fp.layout()

	for i := 0; i < fp.LastW; i++ {
		c.DrawCell(fp.AbsX+i, fp.AbsY, " ", headerBg, fgDim)
	}
	c.DrawTextBounded(fp.AbsX, fp.AbsY, l.nameW, sortLabel("Name", fp.sortField == SortByName, fp.sortDesc), headerBg, fgDim)
	if l.extX >= 0 {
		c.DrawTextBounded(fp.AbsX+l.extX, fp.AbsY, extColWidth, "Ext", headerBg, fgDim)
	}
	if l.sizeX >= 0 {
		c.DrawTextBounded(fp.AbsX+l.sizeX, fp.AbsY, sizeColWidth, sortLabel("Size", fp.sortField == SortBySize, fp.sortDesc), headerBg, fgDim)
	}
	if l.dateX >= 0 {
		c.DrawTextBounded(fp.AbsX+l.dateX, fp.AbsY, dateColWidth, sortLabel("Date", fp.sortField == SortByDate, fp.sortDesc), headerBg, fgDim)
	}
	if l.attrX >= 0 {
		c.DrawTextBounded(fp.AbsX+l.attrX, fp.AbsY, attrColWidth, "Attr", headerBg, fgDim)
	}

	visible := fp.visibleRows()
	for i := 0; i < visible; i++ {
		idx := fp.scroll + i
		y := fp.AbsY + 1 + i
		if idx >= len(fp.rows) {
			for x := 0; x < fp.LastW; x++ {
				c.DrawCell(fp.AbsX+x, y, " ", theme.BgWidget, theme.FgWindow)
			}
			continue
		}
		fp.drawRow(c, y, l, fp.rows[idx], idx == fp.cursor)
	}
	fp.drawScrollbar(c, visible)

	statusY := fp.AbsY + 1 + visible
	for x := 0; x < fp.LastW; x++ {
		c.DrawCell(fp.AbsX+x, statusY, " ", headerBg, fgDim)
	}
	c.DrawTextBounded(fp.AbsX, statusY, fp.LastW, fp.statusLine(), headerBg, fgDim)
}

// drawScrollbar paints a thumb-and-track scrollbar in the listing's
// rightmost column, matching Graphite's own ListBox convention exactly
// (thumb size proportional to the visible/total ratio), so a pane with
// more rows than fit is never silently unscrollable-looking. Draws
// nothing when everything already fits.
func (fp *FilePane) drawScrollbar(c *Graphite.Canvas, visible int) {
	if len(fp.rows) <= visible || fp.LastW < 1 {
		return
	}
	theme := c.Theme()
	thumbH := int(math.Max(1, float64(visible*visible)/float64(len(fp.rows))))
	maxScroll := len(fp.rows) - visible
	thumbY := 0
	if maxScroll > 0 && visible > thumbH {
		thumbY = (fp.scroll * (visible - thumbH)) / maxScroll
	}
	x := fp.AbsX + fp.LastW - 1
	for i := 0; i < visible; i++ {
		y := fp.AbsY + 1 + i
		if i >= thumbY && i < thumbY+thumbH {
			c.DrawCell(x, y, "█", theme.BgWidget, theme.Primary)
		} else {
			c.DrawCell(x, y, "│", theme.BgWidget, theme.FgDisabled)
		}
	}
}

// scrollbarMaxScroll returns how many more rows Scroll can advance,
// shared by drawScrollbar's thumb math and HandleEvent's click-to-jump so
// both agree on the same range.
func (fp *FilePane) scrollbarMaxScroll() int {
	max := len(fp.rows) - fp.visibleRows()
	if max < 0 {
		max = 0
	}
	return max
}

// scrollToClick jumps Scroll based on a click or drag at relY within the
// scrollbar's track (relative to the first listing row), mirroring
// Graphite's own ListBox scrollbar convention: near the thumb's own
// position is a proportional jump, past either end snaps straight there.
func (fp *FilePane) scrollToClick(relY int) {
	visible := fp.visibleRows()
	maxScroll := fp.scrollbarMaxScroll()
	if maxScroll <= 0 {
		return
	}
	thumbH := int(math.Max(1, float64(visible*visible)/float64(len(fp.rows))))

	switch {
	case relY < thumbH/2:
		fp.scroll = 0
	case relY >= visible-thumbH/2 || visible <= thumbH:
		fp.scroll = maxScroll
	default:
		fraction := float64(relY-thumbH/2) / float64(visible-thumbH)
		fp.scroll = int(math.Round(fraction * float64(maxScroll)))
	}

	if fp.scroll < 0 {
		fp.scroll = 0
	}
	if fp.scroll > maxScroll {
		fp.scroll = maxScroll
	}
}

// TaggedSummary returns the count and total size of currently tagged
// entries, and whether anything is tagged at all — exported so a host
// program's own UI (e.g. a status bar showing "N tagged, X MB" for
// whichever pane is focused) can read it without duplicating the tagging
// loop statusLine already does internally.
func (fp *FilePane) TaggedSummary() (count int, size int64, any bool) {
	for _, r := range fp.rows {
		if r.tagged {
			count++
			size += r.Size
		}
	}
	return count, size, count > 0
}

// statusLine summarizes the listing — file/directory counts, and tagged
// count plus total tagged size once anything is tagged — matching the
// per-pane summary a classic commander shows along its own bottom edge.
func (fp *FilePane) statusLine() string {
	var files, dirs int
	for _, r := range fp.rows {
		if r.isParent {
			continue
		}
		if r.IsDir {
			dirs++
		} else {
			files++
		}
	}
	if tagged, size, any := fp.TaggedSummary(); any {
		return fmt.Sprintf("%d file(s), %d dir(s) — %d tagged (%s)", files, dirs, tagged, formatSize(size))
	}
	return fmt.Sprintf("%d file(s), %d dir(s)", files, dirs)
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
	var ext string
	if r.isParent {
		name = ".."
	} else if r.IsDir {
		name = "/" + name
	} else if l.extX >= 0 {
		name, ext = splitExt(r.Name)
	}
	marker := "  "
	if r.tagged {
		marker = "● "
	}
	c.DrawTextBounded(fp.AbsX, y, l.nameW, marker+name, bg, fg)

	if r.isParent {
		return
	}
	if l.extX >= 0 {
		c.DrawTextBounded(fp.AbsX+l.extX, y, extColWidth, ext, bg, fg)
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
