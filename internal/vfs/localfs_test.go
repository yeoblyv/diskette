package vfs

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocalFS_ListSortsNothingButReportsEntries(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "a-sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	fs := LocalFS{}
	entries, err := fs.List(context.Background(), dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("List returned %d entries, want 2", len(entries))
	}

	byName := map[string]Entry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	if !byName["a-sub"].IsDir {
		t.Error("a-sub should be reported as a directory")
	}
	if byName["b.txt"].IsDir {
		t.Error("b.txt should not be reported as a directory")
	}
	if byName["b.txt"].Size != 2 {
		t.Errorf("b.txt size = %d, want 2", byName["b.txt"].Size)
	}
}

func TestLocalFS_MkdirRenameRemove(t *testing.T) {
	dir := t.TempDir()
	fs := LocalFS{}
	ctx := context.Background()

	sub := fs.Join(dir, "created")
	if err := fs.Mkdir(ctx, sub); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if info, err := fs.Stat(ctx, sub); err != nil || !info.IsDir {
		t.Fatalf("Stat after Mkdir = %+v, %v, want an existing directory", info, err)
	}

	renamed := fs.Join(dir, "renamed")
	if err := fs.Rename(ctx, sub, renamed); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := fs.Stat(ctx, sub); err == nil {
		t.Error("old path still exists after Rename")
	}

	if err := fs.Remove(ctx, renamed); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := fs.Stat(ctx, renamed); err == nil {
		t.Error("path still exists after Remove")
	}
}

func TestLocalFS_OpenCreateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	fs := LocalFS{}
	ctx := context.Background()
	path := fs.Join(dir, "data.txt")

	w, err := fs.Create(ctx, path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := w.Write([]byte("payload")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close writer: %v", err)
	}

	r, err := fs.Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()
	buf := make([]byte, 7)
	if _, err := r.Read(buf); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(buf) != "payload" {
		t.Errorf("read %q, want %q", buf, "payload")
	}
}

func TestLocalFS_ParentAtRoot(t *testing.T) {
	fs := LocalFS{}
	root := "/"
	if runtime.GOOS == "windows" {
		root = `C:\`
	}
	if _, ok := fs.Parent(root); ok {
		t.Errorf("Parent(%q) reported a parent at the filesystem root", root)
	}
}
