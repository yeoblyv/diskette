// Package vfs is the storage abstraction the copy engine and the FilePane
// widget operate through, so neither needs to know whether it is talking
// to the local disk or a remote server. LocalFS is the only implementation
// so far; a future SFTPFS will implement the same interface over an
// SSH/SFTP session.
package vfs

import (
	"context"
	"io"
	"time"
)

// Entry describes one file or directory as reported by a FileSystem.
type Entry struct {
	Name    string
	Size    int64
	Mode    string
	ModTime time.Time
	IsDir   bool
}

// FileSystem is the storage contract the copy engine and FilePane operate
// through. All paths are absolute in whatever form this filesystem uses
// (OS-native for LocalFS, POSIX for a future SFTPFS).
type FileSystem interface {
	// List returns the entries directly inside dir.
	List(ctx context.Context, dir string) ([]Entry, error)
	// Stat describes a single path.
	Stat(ctx context.Context, path string) (Entry, error)
	// Mkdir creates a single directory; its parent must already exist.
	Mkdir(ctx context.Context, path string) error
	// Remove deletes a single file or empty directory.
	Remove(ctx context.Context, path string) error
	// Rename moves oldPath to newPath, atomically where the underlying
	// storage supports it.
	Rename(ctx context.Context, oldPath, newPath string) error
	// Open opens path for a streaming read.
	Open(ctx context.Context, path string) (io.ReadCloser, error)
	// Create truncates or creates path for a streaming write.
	Create(ctx context.Context, path string) (io.WriteCloser, error)
	// Join joins path elements using this filesystem's native separator.
	Join(elem ...string) string
	// Parent returns the cleaned parent of path and true, or path and
	// false when path is already a root with no parent.
	Parent(path string) (string, bool)
}
