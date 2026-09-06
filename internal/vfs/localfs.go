package vfs

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/yeoblyv/diskette/internal/pathutil"
)

// LocalFS implements FileSystem over the local OS filesystem via
// path/filepath, so path handling follows the host OS's own separator and
// root convention.
type LocalFS struct{}

// entryFromInfo adapts an os.FileInfo (as returned by both os.Stat and
// os.DirEntry.Info) to the filesystem-agnostic Entry shape.
func entryFromInfo(name string, info os.FileInfo) Entry {
	return Entry{
		Name:    name,
		Size:    info.Size(),
		Mode:    info.Mode().String(),
		ModTime: info.ModTime(),
		IsDir:   info.IsDir(),
	}
}

// List implements FileSystem.
func (LocalFS) List(_ context.Context, dir string) ([]Entry, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(des))
	for _, de := range des {
		info, err := de.Info()
		if err != nil {
			// The entry vanished between ReadDir and Info (e.g. a
			// concurrent delete) — skip it rather than fail the whole
			// listing over one race.
			continue
		}
		entries = append(entries, entryFromInfo(de.Name(), info))
	}
	return entries, nil
}

// Stat implements FileSystem.
func (LocalFS) Stat(_ context.Context, path string) (Entry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Entry{}, err
	}
	return entryFromInfo(info.Name(), info), nil
}

// Mkdir implements FileSystem.
func (LocalFS) Mkdir(_ context.Context, path string) error {
	return os.Mkdir(path, 0o755)
}

// Remove implements FileSystem.
func (LocalFS) Remove(_ context.Context, path string) error {
	return os.Remove(path)
}

// Rename implements FileSystem.
func (LocalFS) Rename(_ context.Context, oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

// Open implements FileSystem.
func (LocalFS) Open(_ context.Context, path string) (io.ReadCloser, error) {
	return os.Open(path)
}

// Create implements FileSystem.
func (LocalFS) Create(_ context.Context, path string) (io.WriteCloser, error) {
	return os.Create(path)
}

// Join implements FileSystem.
func (LocalFS) Join(elem ...string) string {
	return filepath.Join(elem...)
}

// Parent implements FileSystem.
func (LocalFS) Parent(path string) (string, bool) {
	return pathutil.Parent(path)
}
