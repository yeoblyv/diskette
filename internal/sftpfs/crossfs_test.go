package sftpfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeoblyv/diskette/internal/archiveengine"
	"github.com/yeoblyv/diskette/internal/copyengine"
	"github.com/yeoblyv/diskette/internal/vfs"
)

// TestCrossFS_CopyLocalToRemoteAndBack is the actual point of the
// vfs.FileSystem abstraction, proven once explicitly rather than trusted
// by construction: copyengine.Run (untouched, unaware SFTPFS even exists)
// copies a real file from a local directory onto the in-process SFTP
// server, and back again to a second local directory, byte-for-byte.
func TestCrossFS_CopyLocalToRemoteAndBack(t *testing.T) {
	localSrc := t.TempDir()
	remoteRoot := t.TempDir()
	localDst := t.TempDir()

	if err := os.WriteFile(filepath.Join(localSrc, "hello.txt"), []byte("hello from local"), 0o644); err != nil {
		t.Fatal(err)
	}

	remote := clientServerPair(t, remoteRoot)
	local := vfs.LocalFS{}
	ctx := context.Background()

	// Local -> remote.
	if _, err := copyengine.Run(ctx, copyengine.Task{
		SrcFS:    local,
		DstFS:    remote,
		SrcPaths: []string{filepath.Join(localSrc, "hello.txt")},
		DstDir:   ".",
	}, nil, nil); err != nil {
		t.Fatalf("copyengine.Run (local -> remote): %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(remoteRoot, "hello.txt")); err != nil || string(data) != "hello from local" {
		t.Fatalf("remote copy = %q, err = %v, want %q", data, err, "hello from local")
	}

	// Remote -> local (a second local directory).
	if _, err := copyengine.Run(ctx, copyengine.Task{
		SrcFS:    remote,
		DstFS:    local,
		SrcPaths: []string{"hello.txt"},
		DstDir:   localDst,
	}, nil, nil); err != nil {
		t.Fatalf("copyengine.Run (remote -> local): %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(localDst, "hello.txt")); err != nil || string(data) != "hello from local" {
		t.Fatalf("round-tripped copy = %q, err = %v, want %q", data, err, "hello from local")
	}
}

// TestCrossFS_ZipRemoteFilesIntoALocalArchive proves archiveengine.CreateZip
// also works against SFTPFS unmodified: it walks and streams entries
// straight off the remote server into a .zip written to the local disk.
func TestCrossFS_ZipRemoteFilesIntoALocalArchive(t *testing.T) {
	remoteRoot := t.TempDir()
	localDst := t.TempDir()

	if err := os.WriteFile(filepath.Join(remoteRoot, "a.txt"), []byte("aaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(remoteRoot, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remoteRoot, "sub", "b.txt"), []byte("bbb"), 0o644); err != nil {
		t.Fatal(err)
	}

	remote := clientServerPair(t, remoteRoot)
	local := vfs.LocalFS{}
	ctx := context.Background()

	archivePath := filepath.Join(localDst, "out.zip")
	err := archiveengine.CreateZip(ctx, remote, []string{"a.txt", "sub"}, local, archivePath, nil)
	if err != nil {
		t.Fatalf("archiveengine.CreateZip: %v", err)
	}
	if info, statErr := os.Stat(archivePath); statErr != nil || info.Size() == 0 {
		t.Fatalf("archive at %s: stat err = %v, size = %v", archivePath, statErr, info)
	}
}
