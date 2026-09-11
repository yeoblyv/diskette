package grep

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeoblyv/diskette/internal/vfs"
)

// buildTree creates:
//
//	root/
//	  a.txt    ("hello world\nFOO bar\n")
//	  b.go     ("package main\n// hello there\n")
//	  sub/
//	    c.txt  ("nested hello\n")
//	  bin.dat  (contains a NUL byte)
func buildTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"a.txt":     "hello world\nFOO bar\n",
		"b.go":      "package main\n// hello there\n",
		"sub/c.txt": "nested hello\n",
	}
	for rel, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "bin.dat"), []byte("hello\x00world"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runMatches(t *testing.T, dir string, opts Options) []Match {
	t.Helper()
	var got []Match
	if err := Run(context.Background(), vfs.LocalFS{}, dir, opts, func(m Match) {
		got = append(got, m)
	}); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestRun_LiteralCaseInsensitiveByDefault(t *testing.T) {
	dir := buildTree(t)
	got := runMatches(t, dir, Options{Pattern: "hello", Recursive: true})
	if len(got) != 3 { // a.txt, b.go's comment, sub/c.txt — bin.dat skipped as binary
		t.Fatalf("got %d matches, want 3: %+v", len(got), got)
	}
}

func TestRun_LiteralCaseSensitiveExcludesDifferentCase(t *testing.T) {
	dir := buildTree(t)
	got := runMatches(t, dir, Options{Pattern: "FOO", Recursive: true, CaseSensitive: true})
	if len(got) != 1 || got[0].Line != "FOO bar" {
		t.Errorf("got %+v, want exactly the \"FOO bar\" line", got)
	}
	got = runMatches(t, dir, Options{Pattern: "foo", Recursive: true, CaseSensitive: true})
	if len(got) != 0 {
		t.Errorf("got %+v, want no matches (case-sensitive, pattern case differs)", got)
	}
}

func TestRun_NonRecursiveSkipsSubdirectories(t *testing.T) {
	dir := buildTree(t)
	got := runMatches(t, dir, Options{Pattern: "hello", Recursive: false})
	if len(got) != 2 { // a.txt, b.go — sub/c.txt excluded
		t.Fatalf("got %d matches, want 2: %+v", len(got), got)
	}
}

func TestRun_MaskRestrictsSearchedFiles(t *testing.T) {
	dir := buildTree(t)
	got := runMatches(t, dir, Options{Pattern: "hello", Mask: "*.go", Recursive: true})
	if len(got) != 1 || filepath.Base(got[0].Path) != "b.go" {
		t.Errorf("got %+v, want only b.go", got)
	}
}

func TestRun_ReportsCorrectLineNumberAndPath(t *testing.T) {
	dir := buildTree(t)
	got := runMatches(t, dir, Options{Pattern: "there", Recursive: true})
	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1: %+v", len(got), got)
	}
	want := filepath.Join(dir, "b.go")
	if got[0].Path != want || got[0].LineNum != 2 {
		t.Errorf("got %+v, want path=%s line=2", got[0], want)
	}
}

func TestRun_RegexMatches(t *testing.T) {
	dir := buildTree(t)
	got := runMatches(t, dir, Options{Pattern: "^hello", Regex: true, Recursive: true})
	if len(got) != 1 || filepath.Base(got[0].Path) != "a.txt" {
		t.Errorf("got %+v, want only a.txt's \"hello world\" line", got)
	}
}

func TestRun_BadRegexReturnsError(t *testing.T) {
	dir := buildTree(t)
	err := Run(context.Background(), vfs.LocalFS{}, dir, Options{Pattern: "(", Regex: true, Recursive: true}, func(Match) {})
	if err == nil {
		t.Error("Run with a malformed regex returned no error")
	}
}

func TestRun_BadMaskReturnsError(t *testing.T) {
	dir := buildTree(t)
	err := Run(context.Background(), vfs.LocalFS{}, dir, Options{Pattern: "hello", Mask: "[", Recursive: true}, func(Match) {})
	if err == nil {
		t.Error("Run with a malformed mask returned no error")
	}
}

func TestRun_SkipsBinaryFiles(t *testing.T) {
	dir := buildTree(t)
	got := runMatches(t, dir, Options{Pattern: "hello", Mask: "*.dat", Recursive: true})
	if len(got) != 0 {
		t.Errorf("got %+v, want bin.dat skipped as binary", got)
	}
}

func TestRun_RespectsCancellation(t *testing.T) {
	dir := buildTree(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Run(ctx, vfs.LocalFS{}, dir, Options{Pattern: "hello", Recursive: true}, func(Match) {})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run on an already-canceled context returned %v, want context.Canceled", err)
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
	if err := os.WriteFile(filepath.Join(locked, "secret.txt"), []byte("hello secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })

	got := runMatches(t, dir, Options{Pattern: "hello", Recursive: true})
	if len(got) != 3 {
		t.Errorf("got %d matches, want 3 (the unreadable \"locked\" directory skipped, not fatal): %+v", len(got), got)
	}
}
