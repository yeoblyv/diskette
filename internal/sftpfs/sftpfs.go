// Package sftpfs implements vfs.FileSystem over an SFTP session, so a
// FilePane, copyengine, or archiveengine browsing/operating on a remote
// server works exactly the same as it does against vfs.LocalFS — none of
// those packages know or care which one they're talking to.
//
// Unlike LocalFS, an SFTPFS carries real state (an open SSH connection)
// that must be torn down explicitly — see Close, which isn't part of the
// vfs.FileSystem interface itself (that interface has no lifecycle method
// at all, since a local filesystem has nothing to close).
package sftpfs

import (
	"context"
	"io"
	"os"
	"path"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/yeoblyv/diskette/internal/vfs"
)

// SFTPFS implements vfs.FileSystem over one SFTP session. All paths are
// POSIX — every SFTP server's paths are, regardless of what OS diskette
// itself is compiled for, so Join/Parent are built on the stdlib "path"
// package (always "/"-separated, single root), never "path/filepath"
// (which is OS-compile-time-dependent and would emit backslashes on a
// Windows build against what is actually a remote POSIX path).
type SFTPFS struct {
	client *sftp.Client
	conn   *ssh.Client
}

// entryFromInfo adapts an os.FileInfo (as returned by the Client's
// ReadDir/Stat/Lstat) to the filesystem-agnostic vfs.Entry shape — the
// same translation vfs.LocalFS's own entryFromInfo does for os.ReadDir.
func entryFromInfo(name string, info os.FileInfo) vfs.Entry {
	return vfs.Entry{
		Name:    name,
		Size:    info.Size(),
		Mode:    info.Mode().String(),
		ModTime: info.ModTime(),
		IsDir:   info.IsDir(),
	}
}

// List implements vfs.FileSystem. A single entry that fails to describe
// itself doesn't fail the whole listing — matching LocalFS.List's own
// "one bad entry doesn't kill the listing" policy — though in practice
// Client.ReadDir already returns fully-populated os.FileInfo values per
// entry in one round trip, so this is defensive rather than expected.
func (fs *SFTPFS) List(_ context.Context, dir string) ([]vfs.Entry, error) {
	infos, err := fs.client.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]vfs.Entry, 0, len(infos))
	for _, info := range infos {
		if info == nil {
			continue
		}
		entries = append(entries, entryFromInfo(info.Name(), info))
	}
	return entries, nil
}

// Stat implements vfs.FileSystem.
func (fs *SFTPFS) Stat(_ context.Context, path string) (vfs.Entry, error) {
	info, err := fs.client.Stat(path)
	if err != nil {
		return vfs.Entry{}, err
	}
	return entryFromInfo(info.Name(), info), nil
}

// Mkdir implements vfs.FileSystem.
func (fs *SFTPFS) Mkdir(_ context.Context, path string) error {
	return fs.client.Mkdir(path)
}

// Remove implements vfs.FileSystem. Client.Remove already tries both a
// file removal and (on failure) a directory removal, matching this
// interface's "a single file or empty directory" contract in one call.
func (fs *SFTPFS) Remove(_ context.Context, path string) error {
	return fs.client.Remove(path)
}

// Rename implements vfs.FileSystem via the standard SFTP rename (SSH_FXP_RENAME,
// RFC-defined to fail if newPath already exists) rather than the OpenSSH
// posix-rename@openssh.com extension (Client.PosixRename, which overwrites) —
// matching LocalFS.Rename's own os.Rename semantics on a POSIX host as
// closely as the protocol allows, and leaving conflict handling to the
// caller (copyengine already Stats the destination itself before renaming).
func (fs *SFTPFS) Rename(_ context.Context, oldPath, newPath string) error {
	return fs.client.Rename(oldPath, newPath)
}

// Open implements vfs.FileSystem.
func (fs *SFTPFS) Open(_ context.Context, path string) (io.ReadCloser, error) {
	return fs.client.Open(path)
}

// Create implements vfs.FileSystem. Client.Create opens O_RDWR|O_CREATE|
// O_TRUNC, matching os.Create's own truncate-or-create semantics exactly.
func (fs *SFTPFS) Create(_ context.Context, path string) (io.WriteCloser, error) {
	return fs.client.Create(path)
}

// Join implements vfs.FileSystem using the stdlib "path" package — always
// "/"-separated, regardless of host OS.
func (fs *SFTPFS) Join(elem ...string) string {
	return path.Join(elem...)
}

// Parent implements vfs.FileSystem, mirroring internal/pathutil.Parent's
// shape but hardcoded to POSIX's single root ("/") rather than
// path/filepath's OS-dependent one.
func (fs *SFTPFS) Parent(p string) (string, bool) {
	clean := path.Clean(p)
	parent := path.Dir(clean)
	if parent == clean {
		return clean, false
	}
	return parent, true
}

// Close tears down the SFTP session and its underlying SSH connection.
// Not part of vfs.FileSystem (which has no lifecycle method at all) —
// callers that own an SFTPFS's lifetime (a closed tab, the app quitting)
// must call this themselves.
func (fs *SFTPFS) Close() error {
	sftpErr := fs.client.Close()
	connErr := fs.conn.Close()
	if sftpErr != nil {
		return sftpErr
	}
	return connErr
}
