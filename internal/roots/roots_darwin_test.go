//go:build darwin

package roots

import (
	"os"
	"path/filepath"
	"testing"
)

func TestList_AlwaysStartsWithRoot(t *testing.T) {
	got := List()
	if len(got) == 0 || got[0] != "/" {
		t.Fatalf("List() = %v, want it to start with \"/\"", got)
	}
}

func TestVolumesUnder_ListsOnlyDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "External"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "not-a-volume.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := volumesUnder(dir)
	want := []string{dir + "/External"}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("volumesUnder(%q) = %v, want %v", dir, got, want)
	}
}

func TestVolumesUnder_MissingDirReturnsNil(t *testing.T) {
	got := volumesUnder(filepath.Join(t.TempDir(), "does-not-exist"))
	if got != nil {
		t.Errorf("volumesUnder(missing) = %v, want nil", got)
	}
}
