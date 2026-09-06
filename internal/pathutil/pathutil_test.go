package pathutil

import (
	"path/filepath"
	"runtime"
	"testing"
)

// root returns a filesystem root path valid on the current OS, so tests
// exercise real root semantics on Windows (drive root) and POSIX (/)
// instead of a hardcoded separator.
func root() string {
	if runtime.GOOS == "windows" {
		return `C:\`
	}
	return "/"
}

func TestParent(t *testing.T) {
	child := filepath.Join(root(), "a", "b")
	want := filepath.Join(root(), "a")

	got, ok := Parent(child)
	if !ok {
		t.Fatalf("Parent(%q) = (_, false), want (%q, true)", child, want)
	}
	if got != want {
		t.Errorf("Parent(%q) = %q, want %q", child, got, want)
	}
}

func TestParentAtRoot(t *testing.T) {
	r := root()
	if got, ok := Parent(r); ok {
		t.Errorf("Parent(%q) = (%q, true), want ok=false at filesystem root", r, got)
	}
}

func TestIsRoot(t *testing.T) {
	if !IsRoot(root()) {
		t.Errorf("IsRoot(%q) = false, want true", root())
	}
	child := filepath.Join(root(), "a")
	if IsRoot(child) {
		t.Errorf("IsRoot(%q) = true, want false", child)
	}
}
