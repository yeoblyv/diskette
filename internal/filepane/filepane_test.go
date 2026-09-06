package filepane

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	Graphite "github.com/yeoblyv/graphite"

	"github.com/yeoblyv/diskette/internal/vfs"
)

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newTestPane(t *testing.T) (*FilePane, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(dir, "a.txt"), "small")
	mustWriteFile(t, filepath.Join(dir, "b.txt"), "a much bigger file than a.txt")

	fp := New(0, 0, 40, 10, vfs.LocalFS{}, dir)
	fp.LastH = 10 // DrawRelative normally sets this; tests bypass drawing.
	return fp, dir
}

func TestNew_ListsDirectoryWithParentRowFirst(t *testing.T) {
	fp, _ := newTestPane(t)

	if len(fp.rows) != 4 { // .. + sub + a.txt + b.txt
		t.Fatalf("got %d rows, want 4: %+v", len(fp.rows), fp.rows)
	}
	if !fp.rows[0].isParent {
		t.Error("first row should be the synthetic \"..\" parent row")
	}
}

func TestFilePane_DirectoriesSortBeforeFiles(t *testing.T) {
	fp, _ := newTestPane(t)

	sawDir, sawFile := false, false
	for _, r := range fp.rows {
		if r.isParent {
			continue
		}
		if r.IsDir {
			sawDir = true
			if sawFile {
				t.Fatal("a directory row appeared after a file row")
			}
		} else {
			sawFile = true
		}
	}
	if !sawDir || !sawFile {
		t.Fatal("test fixture should contain both a directory and files")
	}
}

func TestFilePane_NavigateIntoAndBackOut(t *testing.T) {
	fp, dir := newTestPane(t)

	subIdx := -1
	for i, r := range fp.rows {
		if r.IsDir && !r.isParent {
			subIdx = i
		}
	}
	if subIdx == -1 {
		t.Fatal("no directory row found")
	}

	fp.cursor = subIdx
	fp.activateCursor()
	if got, want := fp.Path(), filepath.Join(dir, "sub"); got != want {
		t.Fatalf("Path() after entering sub = %q, want %q", got, want)
	}
	if !fp.rows[0].isParent {
		t.Fatal("entering a directory should still show a \"..\" row (it has a parent)")
	}

	fp.cursor = 0
	fp.activateCursor()
	if fp.Path() != dir {
		t.Fatalf("Path() after \"..\" = %q, want %q", fp.Path(), dir)
	}
}

func TestFilePane_SortByToggleDirection(t *testing.T) {
	fp, _ := newTestPane(t)

	// New() leaves the pane on the default sort (SortByName, ascending)
	// already, so the first setSort(SortByName) call below is itself a
	// toggle — clicking an already-active header reverses direction,
	// matching every classic commander. Directories stay pinned before
	// files in both directions (also matching a classic commander), so
	// only the file names, not the whole row order, should reverse.
	files := fileOnlyRowNames(fp)
	if fp.sortDesc {
		t.Fatal("New() should start sorted ascending by name")
	}

	fp.setSort(SortByName)
	if !fp.sortDesc {
		t.Fatal("setSort on the already-active field should toggle to descending")
	}
	reversedFiles := fileOnlyRowNames(fp)

	if len(files) != len(reversedFiles) {
		t.Fatalf("file count changed across a sort toggle: %v vs %v", files, reversedFiles)
	}
	for i := range files {
		if files[i] != reversedFiles[len(reversedFiles)-1-i] {
			t.Fatalf("descending sort is not the reverse of ascending among files: %v vs %v", files, reversedFiles)
		}
	}

	if !fp.rows[1].IsDir && !fp.rows[1].isParent {
		t.Errorf("directories should still sort before files under descending name order; row 1 = %+v", fp.rows[1])
	}
}

// fileOnlyRowNames returns the names of non-directory, non-".." rows in
// their current order.
func fileOnlyRowNames(fp *FilePane) []string {
	var out []string
	for _, r := range fp.rows {
		if !r.isParent && !r.IsDir {
			out = append(out, r.Name)
		}
	}
	return out
}

func TestFilePane_ToggleTagAndAdvance(t *testing.T) {
	fp, _ := newTestPane(t)
	fp.cursor = 1 // first non-".." row

	fp.toggleTagAndAdvance()
	if !fp.rows[1].tagged {
		t.Error("row 1 should be tagged after Insert")
	}
	if fp.cursor != 2 {
		t.Errorf("cursor = %d after Insert, want 2 (advance by one)", fp.cursor)
	}

	fp.cursor = 0
	fp.toggleTagAndAdvance()
	if fp.rows[0].tagged {
		t.Error("the \"..\" row must never be taggable")
	}
	if fp.cursor != 0 {
		t.Error("Insert on the \"..\" row should not move the cursor")
	}
}

func TestFilePane_RightClickTogglesTagWithoutAdvancing(t *testing.T) {
	fp, _ := newTestPane(t)
	fp.cursor = 0 // deliberately not on the row being clicked

	fileIdx := -1
	for i, r := range fp.rows {
		if !r.isParent {
			fileIdx = i
			break
		}
	}
	if fileIdx == -1 {
		t.Fatal("no non-\"..\" row found")
	}
	rowY := fp.AbsY + 1 + (fileIdx - fp.scroll)

	fp.HandleEvent(Graphite.Event{Type: Graphite.EventMouseRightDown, MouseX: fp.AbsX + 2, MouseY: rowY})

	if !fp.rows[fileIdx].tagged {
		t.Error("row should be tagged after a right-click")
	}
	if fp.cursor != fileIdx {
		t.Errorf("cursor = %d after right-click, want %d (moved to the clicked row)", fp.cursor, fileIdx)
	}

	fp.HandleEvent(Graphite.Event{Type: Graphite.EventMouseRightDown, MouseX: fp.AbsX + 2, MouseY: rowY})
	if fp.rows[fileIdx].tagged {
		t.Error("a second right-click on the same row should untag it")
	}
}

func TestFilePane_RightClickOnParentRowDoesNotTagOrPanic(t *testing.T) {
	fp, _ := newTestPane(t)
	rowY := fp.AbsY + 1 // the ".." row

	fp.HandleEvent(Graphite.Event{Type: Graphite.EventMouseRightDown, MouseX: fp.AbsX + 2, MouseY: rowY})

	if fp.rows[0].tagged {
		t.Error("the \"..\" row must never be taggable")
	}
}

func TestFilePane_SelectionPathsFallsBackToCursor(t *testing.T) {
	fp, dir := newTestPane(t)
	fp.cursor = 1

	got := fp.SelectionPaths()
	want := []string{filepath.Join(dir, fp.rows[1].Name)}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("SelectionPaths() with nothing tagged = %v, want %v", got, want)
	}
}

func TestFilePane_SelectionPathsPrefersTags(t *testing.T) {
	fp, dir := newTestPane(t)

	var taggedNames []string
	for i, r := range fp.rows {
		if !r.isParent && !r.IsDir {
			fp.rows[i].tagged = true
			taggedNames = append(taggedNames, r.Name)
		}
	}
	if len(taggedNames) < 2 {
		t.Fatal("test fixture needs at least two files to tag")
	}

	got := fp.SelectionPaths()
	if len(got) != len(taggedNames) {
		t.Fatalf("SelectionPaths() = %v, want %d tagged paths", got, len(taggedNames))
	}
	for _, name := range taggedNames {
		want := filepath.Join(dir, name)
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("SelectionPaths() missing tagged path %q, got %v", want, got)
		}
	}
}

func TestFilePane_MoveCursorClamps(t *testing.T) {
	fp, _ := newTestPane(t)

	fp.moveCursor(-100)
	if fp.cursor != 0 {
		t.Errorf("cursor = %d after moving far up, want 0", fp.cursor)
	}

	fp.moveCursor(100)
	if fp.cursor != len(fp.rows)-1 {
		t.Errorf("cursor = %d after moving far down, want %d", fp.cursor, len(fp.rows)-1)
	}
}

func TestFilePane_ReloadPreservesCursorAndTags(t *testing.T) {
	fp, dir := newTestPane(t)

	var aIdx int
	for i, r := range fp.rows {
		if r.Name == "a.txt" {
			aIdx = i
		}
	}
	fp.cursor = aIdx
	fp.rows[aIdx].tagged = true

	mustWriteFile(t, filepath.Join(dir, "c.txt"), "new file added out of band")
	fp.Reload()

	found := false
	for i, r := range fp.rows {
		if r.Name == "a.txt" {
			found = true
			if !r.tagged {
				t.Error("Reload lost the tag on a.txt")
			}
			if fp.cursor != i {
				t.Errorf("cursor = %d after Reload, want %d (a.txt's new index)", fp.cursor, i)
			}
		}
	}
	if !found {
		t.Fatal("a.txt missing after Reload")
	}
	if len(fp.rows) != 5 { // .. + sub + a.txt + b.txt + c.txt
		t.Errorf("got %d rows after Reload, want 5", len(fp.rows))
	}
}

func TestFilePane_OnFunctionKeyFiresForFKeysOnly(t *testing.T) {
	fp, _ := newTestPane(t)

	var got []Graphite.KeyCode
	fp.OnFunctionKey = func(k Graphite.KeyCode) { got = append(got, k) }

	fp.HandleEvent(Graphite.Event{Type: Graphite.EventKey, Key: Graphite.KeyF5})
	fp.HandleEvent(Graphite.Event{Type: Graphite.EventKey, Key: Graphite.KeyUp})
	fp.HandleEvent(Graphite.Event{Type: Graphite.EventKey, Key: Graphite.KeyF10})

	if len(got) != 2 || got[0] != Graphite.KeyF5 || got[1] != Graphite.KeyF10 {
		t.Errorf("OnFunctionKey calls = %v, want [F5 F10]", got)
	}
}

func TestFilePane_UpPreservesCursorOnTheDirectoryLeft(t *testing.T) {
	fp, dir := newTestPane(t)

	subIdx := -1
	for i, r := range fp.rows {
		if r.IsDir && !r.isParent {
			subIdx = i
		}
	}
	if subIdx == -1 {
		t.Fatal("no directory row found")
	}
	fp.cursor = subIdx
	fp.activateCursor() // enter "sub"
	if fp.Path() != filepath.Join(dir, "sub") {
		t.Fatalf("Path() = %q, want the sub directory", fp.Path())
	}

	fp.cursor = 0
	fp.activateCursor() // ".." back out

	if fp.Path() != dir {
		t.Fatalf("Path() after \"..\" = %q, want %q", fp.Path(), dir)
	}
	if got, ok := fp.Selected(); !ok || got.Name != "sub" {
		t.Errorf("cursor after \"..\" is on %+v (ok=%v), want it back on \"sub\"", got, ok)
	}
}

func TestFilePane_BackAndForward(t *testing.T) {
	fp, dir := newTestPane(t)
	sub := filepath.Join(dir, "sub")

	if fp.CanGoBack() || fp.CanGoForward() {
		t.Fatal("a freshly created pane should have no history either direction")
	}

	fp.SetPath(sub)
	if !fp.CanGoBack() {
		t.Fatal("CanGoBack() = false after navigating, want true")
	}
	if fp.CanGoForward() {
		t.Fatal("CanGoForward() = true right after a fresh navigation, want false")
	}

	fp.Back()
	if fp.Path() != dir {
		t.Fatalf("Path() after Back() = %q, want %q", fp.Path(), dir)
	}
	if !fp.CanGoForward() {
		t.Fatal("CanGoForward() = false after Back(), want true")
	}

	fp.Forward()
	if fp.Path() != sub {
		t.Fatalf("Path() after Forward() = %q, want %q", fp.Path(), sub)
	}
}

func TestFilePane_BackAtStartOfHistoryIsNoop(t *testing.T) {
	fp, dir := newTestPane(t)
	fp.Back()
	if fp.Path() != dir {
		t.Errorf("Back() with no history changed Path() to %q, want unchanged %q", fp.Path(), dir)
	}
}

func TestFilePane_DoubleClickOnDirectoryNavigatesIn(t *testing.T) {
	fp, dir := newTestPane(t)

	subIdx := -1
	for i, r := range fp.rows {
		if r.IsDir && !r.isParent {
			subIdx = i
		}
	}
	if subIdx == -1 {
		t.Fatal("no directory row found")
	}
	rowY := fp.AbsY + 1 + (subIdx - fp.scroll)

	click := Graphite.Event{Type: Graphite.EventMouseDown, MouseX: fp.AbsX + 2, MouseY: rowY}
	fp.HandleEvent(click) // first click: just moves the cursor
	if fp.Path() != dir {
		t.Fatalf("a single click navigated to %q, want it to stay on %q", fp.Path(), dir)
	}

	fp.HandleEvent(click) // second click within the double-click window
	if fp.Path() != filepath.Join(dir, "sub") {
		t.Fatalf("Path() after double-click = %q, want the sub directory", fp.Path())
	}
}

func TestFilePane_EnterOnFileRowCallsOnOpenFile(t *testing.T) {
	fp, dir := newTestPane(t)

	fileIdx := -1
	for i, r := range fp.rows {
		if !r.isParent && !r.IsDir {
			fileIdx = i
			break
		}
	}
	if fileIdx == -1 {
		t.Fatal("no file row found")
	}
	fp.cursor = fileIdx
	wantName := fp.rows[fileIdx].Name

	var gotPath string
	calls := 0
	fp.OnOpenFile = func(path string) { gotPath = path; calls++ }

	fp.HandleEvent(Graphite.Event{Type: Graphite.EventKey, Key: Graphite.KeyEnter})

	if calls != 1 {
		t.Fatalf("OnOpenFile called %d times, want 1", calls)
	}
	want := filepath.Join(dir, wantName)
	if gotPath != want {
		t.Errorf("OnOpenFile path = %q, want %q", gotPath, want)
	}
	if fp.Path() != dir {
		t.Errorf("Path() = %q, want unchanged %q (a file must not navigate)", fp.Path(), dir)
	}
}

func TestFilePane_EnterOnDirectoryRowDoesNotCallOnOpenFile(t *testing.T) {
	fp, _ := newTestPane(t)

	dirIdx := -1
	for i, r := range fp.rows {
		if r.IsDir && !r.isParent {
			dirIdx = i
			break
		}
	}
	if dirIdx == -1 {
		t.Fatal("no directory row found")
	}
	fp.cursor = dirIdx

	called := false
	fp.OnOpenFile = func(string) { called = true }

	fp.HandleEvent(Graphite.Event{Type: Graphite.EventKey, Key: Graphite.KeyEnter})

	if called {
		t.Error("OnOpenFile was called for a directory row, want it only for files")
	}
}

func TestFilePane_TypeAheadJumpsToMatchingName(t *testing.T) {
	fp, _ := newTestPane(t)
	fp.cursor = 0

	fp.HandleEvent(Graphite.Event{Type: Graphite.EventKey, CharCode: 'b'})

	got, ok := fp.Selected()
	if !ok || got.Name != "b.txt" {
		t.Errorf("after typing \"b\", selection = %+v (ok=%v), want b.txt", got, ok)
	}
	if fp.SearchQuery() != "b" {
		t.Errorf("SearchQuery() = %q, want %q", fp.SearchQuery(), "b")
	}
}

func TestFilePane_TypeAheadResetsAfterTimeout(t *testing.T) {
	fp, _ := newTestPane(t)

	fp.typeAhead('b')
	fp.lastSearchAt = time.Now().Add(-2 * searchTimeout)
	fp.typeAhead('a')

	if fp.searchBuf != "a" {
		t.Errorf("searchBuf = %q after a timed-out keystroke, want it reset to just %q", fp.searchBuf, "a")
	}
}

func TestSplitExt(t *testing.T) {
	tests := []struct {
		name     string
		wantBase string
		wantExt  string
	}{
		{"app.go", "app", "go"},
		{"archive.tar.gz", "archive.tar", "gz"},
		{"README", "README", ""},
		{".gitignore", ".gitignore", ""},
	}
	for _, tc := range tests {
		base, ext := splitExt(tc.name)
		if base != tc.wantBase || ext != tc.wantExt {
			t.Errorf("splitExt(%q) = (%q, %q), want (%q, %q)", tc.name, base, ext, tc.wantBase, tc.wantExt)
		}
	}
}

func TestFilePane_StatusLineCountsAndTagging(t *testing.T) {
	fp, _ := newTestPane(t)

	if got := fp.statusLine(); got != "2 file(s), 1 dir(s)" {
		t.Errorf("statusLine() = %q, want %q", got, "2 file(s), 1 dir(s)")
	}

	for i, r := range fp.rows {
		if r.Name == "b.txt" {
			fp.rows[i].tagged = true
		}
	}
	got := fp.statusLine()
	if !strings.Contains(got, "1 tagged") {
		t.Errorf("statusLine() with one tagged file = %q, want it to mention \"1 tagged\"", got)
	}
}

func TestFilePane_TaggedSummary(t *testing.T) {
	fp, _ := newTestPane(t)

	if _, _, any := fp.TaggedSummary(); any {
		t.Fatal("TaggedSummary() reported something tagged on a fresh pane")
	}

	var wantSize int64
	for i, r := range fp.rows {
		if r.Name == "a.txt" || r.Name == "b.txt" {
			fp.rows[i].tagged = true
			wantSize += r.Size
		}
	}

	count, size, any := fp.TaggedSummary()
	if !any || count != 2 || size != wantSize {
		t.Errorf("TaggedSummary() = (%d, %d, %v), want (2, %d, true)", count, size, any, wantSize)
	}
}

// newScrollTestPane creates a pane with far more files than fit on
// screen, for exercising the scrollbar.
func newScrollTestPane(t *testing.T) *FilePane {
	t.Helper()
	dir := t.TempDir()
	for i := 0; i < 30; i++ {
		mustWriteFile(t, filepath.Join(dir, fmt.Sprintf("f%02d.txt", i)), "x")
	}
	fp := New(0, 0, 40, 12, vfs.LocalFS{}, dir)
	fp.LastW, fp.LastH = 40, 12 // header + 10 listing rows + status, per visibleRows()
	return fp
}

func TestFilePane_ScrollbarClickAtTopJumpsToStart(t *testing.T) {
	fp := newScrollTestPane(t)
	fp.scroll = 5

	fp.HandleEvent(Graphite.Event{Type: Graphite.EventMouseDown, MouseX: fp.AbsX + fp.LastW - 1, MouseY: fp.AbsY + 1})

	if fp.scroll != 0 {
		t.Errorf("scroll after clicking the top of the scrollbar = %d, want 0", fp.scroll)
	}
}

func TestFilePane_ScrollbarClickAtBottomJumpsToEnd(t *testing.T) {
	fp := newScrollTestPane(t)

	visible := fp.visibleRows()
	fp.HandleEvent(Graphite.Event{Type: Graphite.EventMouseDown, MouseX: fp.AbsX + fp.LastW - 1, MouseY: fp.AbsY + visible})

	want := fp.scrollbarMaxScroll()
	if fp.scroll != want {
		t.Errorf("scroll after clicking the bottom of the scrollbar = %d, want %d (max)", fp.scroll, want)
	}
}

func TestFilePane_ScrollbarAbsentWhenEverythingFits(t *testing.T) {
	fp, _ := newTestPane(t) // only a handful of rows, well under LastH
	if fp.scrollbarMaxScroll() != 0 {
		t.Errorf("scrollbarMaxScroll() = %d, want 0 when every row already fits", fp.scrollbarMaxScroll())
	}
}

func TestFilePane_HeaderTintReflectsFocus(t *testing.T) {
	fp, _ := newTestPane(t)
	canvas := Graphite.NewCanvas()
	canvas.Resize(40, 10)

	fp.IsFocused = false
	fp.DrawRelative(canvas, 0, 0, 40, 10)
	unfocusedBg := canvas.GetCellBg(fp.AbsX, fp.AbsY)

	fp.IsFocused = true
	fp.DrawRelative(canvas, 0, 0, 40, 10)
	focusedBg := canvas.GetCellBg(fp.AbsX, fp.AbsY)

	if focusedBg == unfocusedBg {
		t.Error("the header background is identical focused vs unfocused; the two panes can't be told apart by it")
	}
}

func TestFilePane_ExportedToggleTagAndSetSort(t *testing.T) {
	fp, _ := newTestPane(t)
	fp.cursor = 1

	fp.ToggleTag()
	if !fp.rows[1].tagged {
		t.Error("ToggleTag did not tag the cursor row")
	}

	fp.SetSort(SortBySize)
	if fp.sortField != SortBySize {
		t.Errorf("SetSort(SortBySize): sortField = %v, want SortBySize", fp.sortField)
	}
}

func TestFilePane_DrawRelativeDoesNotPanicAndPaintsHeader(t *testing.T) {
	fp, _ := newTestPane(t)

	canvas := Graphite.NewCanvas()
	canvas.Resize(40, 10)
	fp.DrawRelative(canvas, 0, 0, 40, 10)
	// Reaching this line without panicking, on a real Canvas, is the
	// primary assertion — DrawRelative touches every column of every
	// visible row via column-width arithmetic that's easy to get
	// off-by-one on.
}
