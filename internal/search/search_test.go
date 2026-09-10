package search

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/yeoblyv/diskette/internal/vfs"
)

// buildTree creates:
//
//	root/
//	  a.txt
//	  b.go
//	  sub/
//	    c.txt
//	    d.go
//
// Deliberately no two entries whose names differ only by case: several
// platforms' default filesystems (macOS's APFS, Windows's NTFS) are
// case-insensitive, so "a.txt" and "A.TXT" would collide into a single
// file rather than coexist as two directory entries.
func buildTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"c.txt", "d.go"} {
		if err := os.WriteFile(filepath.Join(sub, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runNames(t *testing.T, dir string, opts Options) []string {
	t.Helper()
	var got []string
	if err := Run(context.Background(), vfs.LocalFS{}, dir, opts, func(m Match) {
		got = append(got, m.Entry.Name)
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	return got
}

func TestRun_NonRecursiveMatchesOnlyTopLevel(t *testing.T) {
	dir := buildTree(t)
	got := runNames(t, dir, Options{Mask: "*.txt", Recursive: false, CaseSensitive: true})
	want := []string{"a.txt"}
	if !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestRun_RecursiveDescendsIntoSubdirectories(t *testing.T) {
	dir := buildTree(t)
	got := runNames(t, dir, Options{Mask: "*.go", Recursive: true, CaseSensitive: true})
	want := []string{"b.go", "d.go"}
	if !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestRun_CaseInsensitiveByDefault(t *testing.T) {
	dir := buildTree(t)
	// The mask's case differs from the real file's ("a.txt" on disk,
	// "A.TXT" as the search mask) — case-insensitive matching should
	// still find it.
	got := runNames(t, dir, Options{Mask: "A.TXT", Recursive: false, CaseSensitive: false})
	want := []string{"a.txt"}
	if !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestRun_CaseSensitiveExcludesDifferentCase(t *testing.T) {
	dir := buildTree(t)
	got := runNames(t, dir, Options{Mask: "A.TXT", Recursive: false, CaseSensitive: true})
	if len(got) != 0 {
		t.Errorf("got %v, want no matches (case-sensitive, mask case differs from the file)", got)
	}
}

func TestRun_EmptyMaskMatchesEverything(t *testing.T) {
	dir := buildTree(t)
	got := runNames(t, dir, Options{Mask: "", Recursive: false})
	if len(got) != 3 { // a.txt, b.go, sub
		t.Errorf("got %v, want all 3 top-level entries", got)
	}
}

func TestRun_NeverMatchesTheRootItself(t *testing.T) {
	dir := buildTree(t)
	// The root's own base name might itself satisfy a broad mask; Run
	// must not report it as its own match.
	base := filepath.Base(dir)
	got := runNames(t, dir, Options{Mask: base, Recursive: true, CaseSensitive: true})
	if len(got) != 0 {
		t.Errorf("got %v, want no matches (the root must never match itself)", got)
	}
}

func TestRun_MatchPathIsJoinedCorrectly(t *testing.T) {
	dir := buildTree(t)
	var paths []string
	err := Run(context.Background(), vfs.LocalFS{}, dir, Options{Mask: "c.txt", Recursive: true}, func(m Match) {
		paths = append(paths, m.Path)
	})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "sub", "c.txt")
	if len(paths) != 1 || paths[0] != want {
		t.Errorf("paths = %v, want [%s]", paths, want)
	}
}

func TestRun_SkipsAnUnreadableSubdirectoryInsteadOfAborting(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses permission checks")
	}
	dir := buildTree(t)
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "secret.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) }) // so TempDir cleanup can remove it

	got := runNames(t, dir, Options{Mask: "*.go", Recursive: true, CaseSensitive: true})
	want := []string{"b.go", "d.go"}
	if !equal(got, want) {
		t.Errorf("got %v, want %v (the unreadable \"locked\" directory should be skipped, not fatal)", got, want)
	}
}

func TestRun_BadPatternReturnsError(t *testing.T) {
	dir := buildTree(t)
	err := Run(context.Background(), vfs.LocalFS{}, dir, Options{Mask: "[", Recursive: false}, func(Match) {})
	if err == nil {
		t.Error("Run with a malformed mask returned no error")
	}
}

func TestRun_RespectsCancellation(t *testing.T) {
	dir := buildTree(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Run(ctx, vfs.LocalFS{}, dir, Options{Mask: "*", Recursive: true}, func(Match) {})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run on an already-canceled context returned %v, want context.Canceled", err)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
