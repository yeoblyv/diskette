package fileinfo

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStat_ReportsBasicMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "a.txt" {
		t.Errorf("Name = %q, want a.txt", info.Name)
	}
	if info.Size != 5 {
		t.Errorf("Size = %d, want 5", info.Size)
	}
	if info.IsDir {
		t.Error("IsDir = true for a regular file")
	}
	if time.Since(info.Modified) > time.Minute {
		t.Errorf("Modified = %v, want close to now", info.Modified)
	}
}

func TestStat_ReportsDirectory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	info, err := Stat(sub)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir {
		t.Error("IsDir = false for a directory")
	}
}

func TestStat_AccessedIsRecentForAFreshFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(path); err != nil {
		t.Fatal(err)
	}

	info, err := Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Accessed.IsZero() {
		t.Error("Accessed is zero, want a real timestamp on every supported platform")
	}
	if time.Since(info.Accessed) > time.Minute {
		t.Errorf("Accessed = %v, want close to now", info.Accessed)
	}
}

func TestStat_CreatedIsPlausibleWhenKnown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.CreatedKnown {
		t.Skip("this filesystem/kernel doesn't report creation time")
	}
	if time.Since(info.Created) > time.Minute {
		t.Errorf("Created = %v, want close to now", info.Created)
	}
}

func TestStat_NonexistentPathReturnsError(t *testing.T) {
	if _, err := Stat(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("Stat on a missing path returned no error")
	}
}
