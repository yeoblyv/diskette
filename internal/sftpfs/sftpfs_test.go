package sftpfs

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/sftp"

	"github.com/yeoblyv/diskette/internal/vfs"
)

// clientServerPair wires an in-process SFTP client and server together
// over a pair of io.Pipes — the same pattern pkg/sftp's own test suite
// uses (server_test.go's clientServerPair) — so the vfs.FileSystem
// contract is tested against the real wire protocol without any real
// network connection or SSH handshake. root is the local directory the
// in-process server actually serves paths from.
//
// sftp.WithServerWorkingDirectory sets root as the base for *relative*
// path resolution only — it is not a chroot, so every test below uses
// relative paths ("a.txt", not "/a.txt") to stay confined to root; a real
// remote sshd's sftp-server subsystem (what SFTPFS actually talks to in
// production) has no such quirk and genuinely roots "/" at the server's
// own filesystem root, matching vfs.FileSystem's "always absolute"
// contract exactly.
func clientServerPair(t *testing.T, root string) *SFTPFS {
	t.Helper()
	cr, sw := io.Pipe()
	sr, cw := io.Pipe()

	server, err := sftp.NewServer(
		struct {
			io.Reader
			io.WriteCloser
		}{sr, sw},
		sftp.WithServerWorkingDirectory(root),
	)
	if err != nil {
		t.Fatalf("sftp.NewServer: %v", err)
	}
	go server.Serve()

	client, err := sftp.NewClientPipe(cr, cw)
	if err != nil {
		t.Fatalf("sftp.NewClientPipe: %v", err)
	}
	// t.Cleanup runs LIFO, so registering client's cleanup first and
	// server's second makes server.Close() run before client.Close() —
	// the order pkg/sftp's own tests use. The other way around
	// deadlocks: client.Close() waits for its receive-loop goroutine to
	// exit, which only happens once the server side of the pipe closes.
	t.Cleanup(func() { client.Close() })
	t.Cleanup(func() { server.Close() })

	return &SFTPFS{client: client}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSFTPFS_ListAndStat(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a.txt"), "aaa")
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	fs := clientServerPair(t, root)
	ctx := context.Background()

	entries, err := fs.List(ctx, ".")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	byName := map[string]vfs.Entry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	if e, ok := byName["a.txt"]; !ok || e.IsDir || e.Size != 3 {
		t.Errorf("a.txt entry = %+v, ok=%v, want a 3-byte file", e, ok)
	}
	if e, ok := byName["sub"]; !ok || !e.IsDir {
		t.Errorf("sub entry = %+v, ok=%v, want a directory", e, ok)
	}

	stat, err := fs.Stat(ctx, "a.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if stat.Size != 3 || stat.IsDir {
		t.Errorf("Stat(a.txt) = %+v, want a 3-byte file", stat)
	}
}

func TestSFTPFS_MkdirAndRemove(t *testing.T) {
	root := t.TempDir()
	fs := clientServerPair(t, root)
	ctx := context.Background()

	if err := fs.Mkdir(ctx, "newdir"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "newdir")); err != nil {
		t.Fatalf("expected newdir on the real filesystem: %v", err)
	}

	if err := fs.Remove(ctx, "newdir"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "newdir")); !os.IsNotExist(err) {
		t.Errorf("newdir still exists after Remove: err = %v", err)
	}
}

func TestSFTPFS_RemoveAFile(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "gone.txt"), "x")
	fs := clientServerPair(t, root)
	ctx := context.Background()

	if err := fs.Remove(ctx, "gone.txt"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "gone.txt")); !os.IsNotExist(err) {
		t.Errorf("gone.txt still exists after Remove: err = %v", err)
	}
}

func TestSFTPFS_Rename(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "old.txt"), "content")
	fs := clientServerPair(t, root)
	ctx := context.Background()

	if err := fs.Rename(ctx, "old.txt", "new.txt"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "new.txt"))
	if err != nil || string(data) != "content" {
		t.Errorf("new.txt = %q, err = %v, want %q", data, err, "content")
	}
}

func TestSFTPFS_OpenAndCreateRoundTrip(t *testing.T) {
	root := t.TempDir()
	fs := clientServerPair(t, root)
	ctx := context.Background()

	w, err := fs.Create(ctx, "written.txt")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := w.Write([]byte("hello sftp")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close (write side): %v", err)
	}

	r, err := fs.Open(ctx, "written.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "hello sftp" {
		t.Errorf("read back %q, want %q", data, "hello sftp")
	}
}

func TestSFTPFS_JoinIsAlwaysForwardSlash(t *testing.T) {
	fs := &SFTPFS{}
	if got := fs.Join("a", "b", "c"); got != "a/b/c" {
		t.Errorf("Join(a,b,c) = %q, want %q", got, "a/b/c")
	}
}

func TestSFTPFS_ParentAtRootHasNoParent(t *testing.T) {
	fs := &SFTPFS{}
	if got, ok := fs.Parent("/"); ok || got != "/" {
		t.Errorf("Parent(/) = (%q, %v), want (\"/\", false)", got, ok)
	}
}

func TestSFTPFS_ParentOfNestedPath(t *testing.T) {
	fs := &SFTPFS{}
	got, ok := fs.Parent("/a/b/c")
	if !ok || got != "/a/b" {
		t.Errorf("Parent(/a/b/c) = (%q, %v), want (\"/a/b\", true)", got, ok)
	}
}
