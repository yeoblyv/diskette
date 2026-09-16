package copyengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeoblyv/diskette/internal/vfs"
)

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestRun_CopiesFlatFiles(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	mustWriteFile(t, filepath.Join(src, "a.txt"), "aaa")
	mustWriteFile(t, filepath.Join(src, "b.txt"), "bbb")

	task := Task{
		SrcFS:    vfs.LocalFS{},
		DstFS:    vfs.LocalFS{},
		SrcPaths: []string{filepath.Join(src, "a.txt"), filepath.Join(src, "b.txt")},
		DstDir:   dst,
	}

	done, err := Run(context.Background(), task, nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if done != 2 {
		t.Errorf("Run returned %d, want 2", done)
	}
	if got := mustReadFile(t, filepath.Join(dst, "a.txt")); got != "aaa" {
		t.Errorf("a.txt = %q, want aaa", got)
	}
	if got := mustReadFile(t, filepath.Join(dst, "b.txt")); got != "bbb" {
		t.Errorf("b.txt = %q, want bbb", got)
	}
	if !exists(filepath.Join(src, "a.txt")) {
		t.Error("plain Copy must not remove the source")
	}
}

func TestRun_CopiesDirectoryRecursively(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "proj", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(src, "proj", "top.txt"), "top")
	mustWriteFile(t, filepath.Join(src, "proj", "sub", "nested.txt"), "nested")

	task := Task{
		SrcFS:    vfs.LocalFS{},
		DstFS:    vfs.LocalFS{},
		SrcPaths: []string{filepath.Join(src, "proj")},
		DstDir:   dst,
	}

	if _, err := Run(context.Background(), task, nil, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := mustReadFile(t, filepath.Join(dst, "proj", "top.txt")); got != "top" {
		t.Errorf("top.txt = %q, want top", got)
	}
	if got := mustReadFile(t, filepath.Join(dst, "proj", "sub", "nested.txt")); got != "nested" {
		t.Errorf("nested.txt = %q, want nested", got)
	}
}

func TestRun_MoveRemovesSource(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "proj", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(src, "proj", "sub", "f.txt"), "x")

	task := Task{
		SrcFS:    vfs.LocalFS{},
		DstFS:    vfs.LocalFS{},
		SrcPaths: []string{filepath.Join(src, "proj")},
		DstDir:   dst,
		Move:     true,
	}

	if _, err := Run(context.Background(), task, nil, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if exists(filepath.Join(src, "proj")) {
		t.Error("Move must remove the source tree once it copied cleanly")
	}
	if got := mustReadFile(t, filepath.Join(dst, "proj", "sub", "f.txt")); got != "x" {
		t.Errorf("moved file = %q, want x", got)
	}
}

// Regression: copying a file to the exact same path on the exact same
// filesystem — the common way to reach this in the UI is both panes
// pointed at the same directory — used to destroy its content instead of
// leaving it untouched. copyFile opens the destination for writing
// (os.Create truncates) while a *different* handle is still reading the
// identical underlying file, so the read side saw EOF immediately and the
// "copy" finished successfully having written zero bytes — silently
// replacing the file with an empty one. This is the single most severe
// bug this package has had: it destroys data during what looks like (and
// the UI presents as) an entirely routine operation, with no warning.
func TestRun_CopyOntoItselfDoesNotDestroyContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "important.txt")
	const original = "this data must survive a self-copy"
	mustWriteFile(t, path, original)

	task := Task{
		SrcFS:    vfs.LocalFS{},
		DstFS:    vfs.LocalFS{},
		SrcPaths: []string{path},
		DstDir:   dir, // the file's own directory — src and dst resolve to the same path
		Move:     false,
	}

	n, err := Run(context.Background(), task, nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n != 1 {
		t.Errorf("Run reported %d done, want 1 (the file counts as already being in place)", n)
	}
	if got := mustReadFile(t, path); got != original {
		t.Errorf("CONTENT DESTROYED: file now contains %q (%d bytes), want the original %q (%d bytes)", got, len(got), original, len(original))
	}
}

// Regression: same root cause as TestRun_CopyOntoItselfDoesNotDestroyContent,
// but for Move — strictly worse, since after copyFile silently emptied the
// file, Move's own "delete the source once it copied cleanly" step would
// then remove even that empty husk, destroying the file with no trace left
// at all (not even a truncated remnant to notice something went wrong).
func TestRun_MoveOntoItselfDoesNotDestroyOrRemoveTheFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "important.txt")
	const original = "this data must survive a self-move"
	mustWriteFile(t, path, original)

	task := Task{
		SrcFS:    vfs.LocalFS{},
		DstFS:    vfs.LocalFS{},
		SrcPaths: []string{path},
		DstDir:   dir,
		Move:     true,
	}

	if _, err := Run(context.Background(), task, nil, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !exists(path) {
		t.Fatal("FILE DELETED: a self-move removed the only copy of the file")
	}
	if got := mustReadFile(t, path); got != original {
		t.Errorf("CONTENT DESTROYED: file now contains %q (%d bytes), want the original %q (%d bytes)", got, len(got), original, len(original))
	}
}

// Regression: the same self-collision, several directory levels deep — a
// whole tree copied onto itself must protect every file it contains, not
// just a top-level one, since plan() recurses and each descendant hits
// the identical src==dst condition independently.
func TestRun_CopyDirectoryOntoItselfDoesNotDestroyNestedFiles(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "proj")
	if err := os.MkdirAll(filepath.Join(proj, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(proj, "top.txt"), "top-level data")
	mustWriteFile(t, filepath.Join(proj, "sub", "nested.txt"), "nested data")

	task := Task{
		SrcFS:    vfs.LocalFS{},
		DstFS:    vfs.LocalFS{},
		SrcPaths: []string{proj},
		DstDir:   dir, // proj's own parent — copying proj onto itself
		Move:     false,
	}

	if _, err := Run(context.Background(), task, nil, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := mustReadFile(t, filepath.Join(proj, "top.txt")); got != "top-level data" {
		t.Errorf("CONTENT DESTROYED: top.txt = %q, want %q", got, "top-level data")
	}
	if got := mustReadFile(t, filepath.Join(proj, "sub", "nested.txt")); got != "nested data" {
		t.Errorf("CONTENT DESTROYED: sub/nested.txt = %q, want %q", got, "nested data")
	}
}

// Same as TestRun_CopyDirectoryOntoItselfDoesNotDestroyNestedFiles, but
// for Move — the combination of both severe cases at once: an entire
// directory tree "moved" onto itself must leave every file's content
// intact *and* leave the whole tree in place, not partially deleted by
// the end-of-run cleanup pass that removes each un-skipped root.
func TestRun_MoveDirectoryOntoItselfDoesNotDestroyOrRemoveAnything(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "proj")
	if err := os.MkdirAll(filepath.Join(proj, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(proj, "top.txt"), "top-level data")
	mustWriteFile(t, filepath.Join(proj, "sub", "nested.txt"), "nested data")

	task := Task{
		SrcFS:    vfs.LocalFS{},
		DstFS:    vfs.LocalFS{},
		SrcPaths: []string{proj},
		DstDir:   dir,
		Move:     true,
	}

	if _, err := Run(context.Background(), task, nil, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !exists(proj) {
		t.Fatal("TREE DELETED: a self-move removed the directory being moved onto itself")
	}
	if got := mustReadFile(t, filepath.Join(proj, "top.txt")); got != "top-level data" {
		t.Errorf("CONTENT DESTROYED: top.txt = %q, want %q", got, "top-level data")
	}
	if got := mustReadFile(t, filepath.Join(proj, "sub", "nested.txt")); got != "nested data" {
		t.Errorf("CONTENT DESTROYED: sub/nested.txt = %q, want %q", got, "nested data")
	}
}

// Regression: this is the single worst bug this package has had. Moving
// a directory into one of its own subdirectories used to succeed with no
// error at all, while silently destroying every trace of the data — the
// copy phase placed a fresh copy inside the tree being moved, and the
// end-of-run cleanup then deleted the *entire* source root (unaware the
// destination was ever inside it), taking the just-created copy down
// with it. Both the original and the "moved" copy were gone, and Run
// reported success. Must be refused outright before touching anything.
func TestRun_MoveDirectoryIntoOwnSubdirectoryIsRefused(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(a, "b")
	if err := os.MkdirAll(b, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(a, "important.txt"), "critical data")

	task := Task{
		SrcFS:    vfs.LocalFS{},
		DstFS:    vfs.LocalFS{},
		SrcPaths: []string{a},
		DstDir:   b, // b is inside a — moving a into its own subdirectory
		Move:     true,
	}

	_, err := Run(context.Background(), task, nil, nil)
	if !errors.Is(err, ErrDestinationInsideSource) {
		t.Fatalf("Run error = %v, want ErrDestinationInsideSource", err)
	}

	// Nothing must have been touched: not moved, not copied, not deleted.
	if !exists(a) {
		t.Error("DATA LOST: the source directory is gone")
	}
	if got := mustReadFile(t, filepath.Join(a, "important.txt")); got != "critical data" {
		t.Errorf("CONTENT DESTROYED: important.txt = %q, want %q", got, "critical data")
	}
	if exists(filepath.Join(b, "a")) {
		t.Error("a partial copy was created inside the destination despite the operation being refused")
	}
}

// Same guard, for Copy: not destructive the way Move is (nothing gets
// deleted afterward), but "copy a folder into its own subfolder" is just
// as nonsensical, so it's refused the same way rather than silently
// producing a self-nested copy.
func TestRun_CopyDirectoryIntoOwnSubdirectoryIsRefused(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(a, "b")
	if err := os.MkdirAll(b, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(a, "important.txt"), "critical data")

	task := Task{
		SrcFS:    vfs.LocalFS{},
		DstFS:    vfs.LocalFS{},
		SrcPaths: []string{a},
		DstDir:   b,
		Move:     false,
	}

	_, err := Run(context.Background(), task, nil, nil)
	if !errors.Is(err, ErrDestinationInsideSource) {
		t.Fatalf("Run error = %v, want ErrDestinationInsideSource", err)
	}
	if exists(filepath.Join(b, "a")) {
		t.Error("a nested copy was created despite the operation being refused")
	}
}

// The exact-equal case: moving/copying a directory "into itself" by
// name, with DstDir equal to the source root's own path rather than a
// subdirectory of it.
func TestRun_MoveDirectoryOntoItsOwnPathIsRefused(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	if err := os.MkdirAll(a, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(a, "f.txt"), "data")

	task := Task{
		SrcFS:    vfs.LocalFS{},
		DstFS:    vfs.LocalFS{},
		SrcPaths: []string{a},
		DstDir:   a,
		Move:     true,
	}

	_, err := Run(context.Background(), task, nil, nil)
	if !errors.Is(err, ErrDestinationInsideSource) {
		t.Fatalf("Run error = %v, want ErrDestinationInsideSource", err)
	}
	if got := mustReadFile(t, filepath.Join(a, "f.txt")); got != "data" {
		t.Errorf("CONTENT DESTROYED: f.txt = %q, want %q", got, "data")
	}
}

// A destination that merely *starts with* the same characters as a source
// root, without actually being inside it (a sibling directory with a
// longer name that happens to share a prefix), must not be refused — the
// guard has to check for a real path-separator boundary, not just a
// string prefix match.
func TestRun_SiblingWithSharedNamePrefixIsNotRefused(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos")
	photosOld := filepath.Join(dir, "photos-old")
	if err := os.MkdirAll(photos, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(photosOld, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(photos, "f.txt"), "data")

	task := Task{
		SrcFS:    vfs.LocalFS{},
		DstFS:    vfs.LocalFS{},
		SrcPaths: []string{photos},
		DstDir:   photosOld,
		Move:     false,
	}

	if _, err := Run(context.Background(), task, nil, nil); err != nil {
		t.Fatalf("Run: %v (a same-prefix sibling must not be treated as nested)", err)
	}
	if got := mustReadFile(t, filepath.Join(photosOld, "photos", "f.txt")); got != "data" {
		t.Errorf("copied file = %q, want %q", got, "data")
	}
}

func TestRun_ConflictOverwrite(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	mustWriteFile(t, filepath.Join(src, "a.txt"), "new")
	mustWriteFile(t, filepath.Join(dst, "a.txt"), "old")

	task := Task{
		SrcFS:    vfs.LocalFS{},
		DstFS:    vfs.LocalFS{},
		SrcPaths: []string{filepath.Join(src, "a.txt")},
		DstDir:   dst,
	}

	asked := false
	_, err := Run(context.Background(), task, nil, func(c Conflict) Resolution {
		asked = true
		return Resolution{Action: Overwrite}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !asked {
		t.Error("onConflict was never called for a colliding file")
	}
	if got := mustReadFile(t, filepath.Join(dst, "a.txt")); got != "new" {
		t.Errorf("a.txt = %q, want new (overwritten)", got)
	}
}

func TestRun_ConflictSkipSkipsWholeSubtree(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "proj", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(src, "proj", "sub", "f.txt"), "new")
	if err := os.Mkdir(filepath.Join(dst, "proj"), 0o755); err != nil {
		t.Fatal(err)
	}

	task := Task{
		SrcFS:    vfs.LocalFS{},
		DstFS:    vfs.LocalFS{},
		SrcPaths: []string{filepath.Join(src, "proj")},
		DstDir:   dst,
	}

	done, err := Run(context.Background(), task, nil, func(c Conflict) Resolution {
		return Resolution{Action: Skip}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if done != 0 {
		t.Errorf("Run returned %d, want 0 (everything skipped)", done)
	}
	if exists(filepath.Join(dst, "proj", "sub")) {
		t.Error("Skip on the colliding directory should skip its whole subtree, not just itself")
	}
}

func TestRun_ConflictRename(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	mustWriteFile(t, filepath.Join(src, "a.txt"), "new")
	mustWriteFile(t, filepath.Join(dst, "a.txt"), "old")

	task := Task{
		SrcFS:    vfs.LocalFS{},
		DstFS:    vfs.LocalFS{},
		SrcPaths: []string{filepath.Join(src, "a.txt")},
		DstDir:   dst,
	}

	_, err := Run(context.Background(), task, nil, func(c Conflict) Resolution {
		return Resolution{Action: Rename, NewName: "a-copy.txt"}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := mustReadFile(t, filepath.Join(dst, "a.txt")); got != "old" {
		t.Errorf("original a.txt = %q, want untouched old", got)
	}
	if got := mustReadFile(t, filepath.Join(dst, "a-copy.txt")); got != "new" {
		t.Errorf("a-copy.txt = %q, want new", got)
	}
}

func TestRun_ConflictCancelStopsEarly(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	mustWriteFile(t, filepath.Join(src, "a.txt"), "a")
	mustWriteFile(t, filepath.Join(src, "b.txt"), "b")
	mustWriteFile(t, filepath.Join(dst, "a.txt"), "existing")

	task := Task{
		SrcFS:    vfs.LocalFS{},
		DstFS:    vfs.LocalFS{},
		SrcPaths: []string{filepath.Join(src, "a.txt"), filepath.Join(src, "b.txt")},
		DstDir:   dst,
	}

	_, err := Run(context.Background(), task, nil, func(c Conflict) Resolution {
		return Resolution{Action: Cancel}
	})
	if err != ErrCanceledByUser {
		t.Errorf("Run error = %v, want ErrCanceledByUser", err)
	}
	if exists(filepath.Join(dst, "b.txt")) {
		t.Error("Cancel should stop before processing later items")
	}
}

func TestRun_ForAllAsksOnlyOnce(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	mustWriteFile(t, filepath.Join(src, "a.txt"), "a-new")
	mustWriteFile(t, filepath.Join(src, "b.txt"), "b-new")
	mustWriteFile(t, filepath.Join(dst, "a.txt"), "a-old")
	mustWriteFile(t, filepath.Join(dst, "b.txt"), "b-old")

	task := Task{
		SrcFS:    vfs.LocalFS{},
		DstFS:    vfs.LocalFS{},
		SrcPaths: []string{filepath.Join(src, "a.txt"), filepath.Join(src, "b.txt")},
		DstDir:   dst,
	}

	askCount := 0
	_, err := Run(context.Background(), task, nil, func(c Conflict) Resolution {
		askCount++
		return Resolution{Action: Overwrite, ForAll: true}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if askCount != 1 {
		t.Errorf("onConflict called %d times, want 1 (ForAll should suppress the rest)", askCount)
	}
	if got := mustReadFile(t, filepath.Join(dst, "b.txt")); got != "b-new" {
		t.Errorf("b.txt = %q, want b-new (ForAll should have applied Overwrite)", got)
	}
}

func TestRun_ContextCancellationStopsEarly(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	mustWriteFile(t, filepath.Join(src, "a.txt"), "a")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	task := Task{
		SrcFS:    vfs.LocalFS{},
		DstFS:    vfs.LocalFS{},
		SrcPaths: []string{filepath.Join(src, "a.txt")},
		DstDir:   dst,
	}

	_, err := Run(ctx, task, nil, nil)
	if err == nil {
		t.Error("Run with an already-canceled context should return an error")
	}
}

func TestRemoveAll_DeletesFileAndDirectoryTree(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "solo.txt"), "x")

	if err := RemoveAll(context.Background(), vfs.LocalFS{}, filepath.Join(root, "solo.txt")); err != nil {
		t.Fatalf("RemoveAll(file): %v", err)
	}
	if exists(filepath.Join(root, "solo.txt")) {
		t.Error("file still exists after RemoveAll")
	}

	if err := os.MkdirAll(filepath.Join(root, "tree", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(root, "tree", "top.txt"), "a")
	mustWriteFile(t, filepath.Join(root, "tree", "sub", "nested.txt"), "b")

	if err := RemoveAll(context.Background(), vfs.LocalFS{}, filepath.Join(root, "tree")); err != nil {
		t.Fatalf("RemoveAll(dir): %v", err)
	}
	if exists(filepath.Join(root, "tree")) {
		t.Error("directory tree still exists after RemoveAll")
	}
}

func TestRun_ProgressReportsFinalCall(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	mustWriteFile(t, filepath.Join(src, "a.txt"), "a")

	task := Task{
		SrcFS:    vfs.LocalFS{},
		DstFS:    vfs.LocalFS{},
		SrcPaths: []string{filepath.Join(src, "a.txt")},
		DstDir:   dst,
	}

	var last Progress
	calls := 0
	_, err := Run(context.Background(), task, func(p Progress) {
		calls++
		last = p
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls == 0 {
		t.Fatal("onProgress was never called")
	}
	if last.FilesDone != 1 || last.FilesTotal != 1 {
		t.Errorf("final progress = %+v, want FilesDone=1 FilesTotal=1", last)
	}
}
