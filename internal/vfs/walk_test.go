package vfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestWalk_VisitsDirectoryBeforeChildrenAndEveryEntry(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "top.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "nested.txt"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}

	fs := LocalFS{}
	var visited []string
	err := Walk(context.Background(), fs, root, func(path string, _ Entry) error {
		visited = append(visited, path)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	want := []string{
		root,
		filepath.Join(root, "sub"),
		filepath.Join(root, "sub", "nested.txt"),
		filepath.Join(root, "top.txt"),
	}
	gotSorted := append([]string(nil), visited...)
	sort.Strings(gotSorted)
	sort.Strings(want)
	if len(gotSorted) != len(want) {
		t.Fatalf("Walk visited %v, want %v", gotSorted, want)
	}
	for i := range want {
		if gotSorted[i] != want[i] {
			t.Errorf("Walk visited %v, want %v", gotSorted, want)
			break
		}
	}

	// The directory itself must precede its own children in visitation
	// order, since a copy relies on this to create it before writing in.
	dirIdx, childIdx := -1, -1
	for i, p := range visited {
		if p == filepath.Join(root, "sub") {
			dirIdx = i
		}
		if p == filepath.Join(root, "sub", "nested.txt") {
			childIdx = i
		}
	}
	if dirIdx == -1 || childIdx == -1 || dirIdx > childIdx {
		t.Errorf("expected %q before %q in %v", filepath.Join(root, "sub"), filepath.Join(root, "sub", "nested.txt"), visited)
	}
}

func TestWalk_StopsOnCallbackError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	boom := errors.New("boom")
	err := Walk(context.Background(), LocalFS{}, root, func(path string, _ Entry) error {
		if filepath.Base(path) == "a.txt" {
			return boom
		}
		return nil
	})
	if !errors.Is(err, boom) {
		t.Errorf("Walk error = %v, want %v", err, boom)
	}
}
