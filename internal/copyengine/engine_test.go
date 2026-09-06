package copyengine

import (
	"context"
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
