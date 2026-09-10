// Package ftpfs implements vfs.FileSystem over FTP/FTPS
// (github.com/jlaffaye/ftp), so a FilePane, copyengine, or archiveengine
// browsing/operating on an FTP server works exactly the same as it does
// against vfs.LocalFS or the sibling internal/sftpfs package — none of
// those packages know or care which one they're talking to.
//
// Unlike LocalFS, an FTPFS carries real state (an open control connection)
// that must be torn down explicitly — see Close, which isn't part of the
// vfs.FileSystem interface itself (that interface has no lifecycle method
// at all, since a local filesystem has nothing to close).
package ftpfs

import (
	"context"
	"fmt"
	"io"
	"path"
	"sync"

	"github.com/jlaffaye/ftp"

	"github.com/yeoblyv/diskette/internal/vfs"
)

// FTPFS implements vfs.FileSystem over one FTP control connection. All
// paths are POSIX — an FTP server's paths aren't OS-dependent any more
// than an SFTP server's are — so Join/Parent are built on the stdlib
// "path" package, matching internal/sftpfs's own reasoning exactly.
//
// A *ftp.ServerConn "supports one in-flight data connection" and "is not
// safe to be called concurrently" (per its own doc comment) — mu
// serializes every method below onto it, the same role the mutex would
// play even if this package had rolled the FTP protocol by hand instead
// of using the library.
type FTPFS struct {
	mu   sync.Mutex
	conn *ftp.ServerConn
}

// entryFromFTP adapts a *ftp.Entry (as returned by List/GetEntry) to the
// filesystem-agnostic vfs.Entry shape.
func entryFromFTP(e *ftp.Entry) vfs.Entry {
	return vfs.Entry{
		Name:    e.Name,
		Size:    int64(e.Size),
		Mode:    e.Type.String(),
		ModTime: e.Time,
		IsDir:   e.Type == ftp.EntryTypeFolder,
	}
}

// List implements vfs.FileSystem. The library's own List already tries
// MLSD (machine-readable, RFC 3659) and falls back to classic LIST
// parsing for servers old enough not to support it — nothing here needs
// to know which one actually happened.
func (fs *FTPFS) List(_ context.Context, dir string) ([]vfs.Entry, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	ftpEntries, err := fs.conn.List(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]vfs.Entry, 0, len(ftpEntries))
	for _, e := range ftpEntries {
		if e.Name == "." || e.Name == ".." {
			continue
		}
		entries = append(entries, entryFromFTP(e))
	}
	return entries, nil
}

// Stat implements vfs.FileSystem. FTP has no universal single-entry stat
// at the wire level for every server, but RFC 3659's MLST does (exposed
// here as the library's GetEntry) and is supported by essentially every
// modern FTP server; the fallback — listing path's own parent and
// matching by name — covers the rest, at the cost of an extra round trip.
func (fs *FTPFS) Stat(ctx context.Context, p string) (vfs.Entry, error) {
	fs.mu.Lock()
	e, err := fs.conn.GetEntry(p)
	fs.mu.Unlock()
	if err == nil {
		return entryFromFTP(e), nil
	}

	if p == "/" {
		// No parent to List instead — the root always exists and is
		// always a directory.
		return vfs.Entry{Name: "/", IsDir: true}, nil
	}
	parent, _ := fs.Parent(p)
	name := path.Base(p)
	entries, listErr := fs.List(ctx, parent)
	if listErr != nil {
		return vfs.Entry{}, err // the original GetEntry error is the more informative one
	}
	for _, entry := range entries {
		if entry.Name == name {
			return entry, nil
		}
	}
	return vfs.Entry{}, fmt.Errorf("ftpfs: %s: not found", p)
}

// Mkdir implements vfs.FileSystem.
func (fs *FTPFS) Mkdir(_ context.Context, p string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.conn.MakeDir(p)
}

// Remove implements vfs.FileSystem: deletes a file, or an empty
// directory if p turns out to be one — matching the interface's own "a
// single file or empty directory" contract the same way
// sftpfs.SFTPFS.Remove does.
func (fs *FTPFS) Remove(ctx context.Context, p string) error {
	fs.mu.Lock()
	err := fs.conn.Delete(p)
	fs.mu.Unlock()
	if err == nil {
		return nil
	}

	entry, statErr := fs.Stat(ctx, p)
	if statErr == nil && entry.IsDir {
		fs.mu.Lock()
		defer fs.mu.Unlock()
		return fs.conn.RemoveDir(p)
	}
	return err
}

// Rename implements vfs.FileSystem.
func (fs *FTPFS) Rename(_ context.Context, oldPath, newPath string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.conn.Rename(oldPath, newPath)
}

// Open implements vfs.FileSystem. The library's *ftp.Response already
// implements io.Reader and io.Closer.
func (fs *FTPFS) Open(_ context.Context, p string) (io.ReadCloser, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.conn.Retr(p)
}

// Create implements vfs.FileSystem. The library's Stor(path, io.Reader)
// takes a reader and blocks until the whole upload finishes — the
// opposite shape from the io.WriteCloser this interface needs to hand
// back immediately for the caller to stream into. Bridged with an
// io.Pipe, exactly as Stor's own doc comment suggests ("io.Pipe() can be
// used if an io.Writer is required"): storeWriter returns the pipe's
// write end right away, while Stor drains the read end on a background
// goroutine and reports its outcome back through storeWriter's Close.
func (fs *FTPFS) Create(_ context.Context, p string) (io.WriteCloser, error) {
	pr, pw := io.Pipe()
	done := make(chan error, 1)

	go func() {
		fs.mu.Lock()
		defer fs.mu.Unlock()
		done <- fs.conn.Stor(p, pr)
	}()

	return &storeWriter{pw: pw, pr: pr, done: done}, nil
}

// storeWriter is Create's io.WriteCloser: writes go straight to the pipe,
// and Close waits for the background Stor call to actually finish (and
// surfaces its error) rather than returning as soon as the last byte is
// buffered — a caller checking Create's returned error only from Close
// (the normal io.WriteCloser contract) would otherwise never learn the
// upload itself failed.
type storeWriter struct {
	pw   *io.PipeWriter
	pr   *io.PipeReader
	done chan error
}

func (w *storeWriter) Write(p []byte) (int, error) { return w.pw.Write(p) }

func (w *storeWriter) Close() error {
	if err := w.pw.Close(); err != nil {
		return err
	}
	return <-w.done
}

// Join implements vfs.FileSystem using the stdlib "path" package — always
// "/"-separated, regardless of host OS.
func (fs *FTPFS) Join(elem ...string) string {
	return path.Join(elem...)
}

// Parent implements vfs.FileSystem, mirroring sftpfs.SFTPFS.Parent's
// shape — POSIX's single root ("/"), not path/filepath's OS-dependent
// one.
func (fs *FTPFS) Parent(p string) (string, bool) {
	clean := path.Clean(p)
	parent := path.Dir(clean)
	if parent == clean {
		return clean, false
	}
	return parent, true
}

// Close ends the FTP session (QUIT) and closes the control connection.
// Not part of vfs.FileSystem (which has no lifecycle method at all) —
// callers that own an FTPFS's lifetime (a closed tab, the app quitting)
// must call this themselves, the same contract sftpfs.SFTPFS.Close has.
func (fs *FTPFS) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.conn.Quit()
}
