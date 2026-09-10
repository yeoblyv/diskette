package ftpfs

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func dialTestServer(t *testing.T, root string, tlsMode TLSMode) *FTPFS {
	t.Helper()
	srv := newTestFTPServer(t, root, "tester", "secret")
	host, port := splitAddr(t, srv.addr())

	cfg := Config{Host: host, Port: port, Username: "tester", Password: "secret", TLSMode: tlsMode}
	if tlsMode != TLSNone {
		cfg.TLSConfig = srv.clientTLSConfig()
	}
	fs, err := Dial(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { fs.Close() })
	return fs
}

func splitAddr(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("splitting %q: %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parsing port %q: %v", portStr, err)
	}
	return host, port
}

func TestFTPFS_ListAndStat(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a.txt"), "aaa")
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	fs := dialTestServer(t, root, TLSNone)
	ctx := context.Background()

	entries, err := fs.List(ctx, "/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	byName := map[string]bool{}
	sizes := map[string]int64{}
	dirs := map[string]bool{}
	for _, e := range entries {
		byName[e.Name] = true
		sizes[e.Name] = e.Size
		dirs[e.Name] = e.IsDir
	}
	if !byName["a.txt"] || sizes["a.txt"] != 3 || dirs["a.txt"] {
		t.Errorf("a.txt entry wrong: present=%v size=%d isDir=%v", byName["a.txt"], sizes["a.txt"], dirs["a.txt"])
	}
	if !byName["sub"] || !dirs["sub"] {
		t.Errorf("sub entry wrong: present=%v isDir=%v", byName["sub"], dirs["sub"])
	}

	stat, err := fs.Stat(ctx, "/a.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if stat.Size != 3 || stat.IsDir {
		t.Errorf("Stat(/a.txt) = %+v, want a 3-byte file", stat)
	}

	dirStat, err := fs.Stat(ctx, "/sub")
	if err != nil {
		t.Fatalf("Stat(/sub): %v", err)
	}
	if !dirStat.IsDir {
		t.Errorf("Stat(/sub).IsDir = false, want true")
	}
}

func TestFTPFS_StatRoot(t *testing.T) {
	root := t.TempDir()
	fs := dialTestServer(t, root, TLSNone)
	entry, err := fs.Stat(context.Background(), "/")
	if err != nil {
		t.Fatalf("Stat(/): %v", err)
	}
	if !entry.IsDir {
		t.Error("Stat(/).IsDir = false, want true")
	}
}

func TestFTPFS_MkdirAndRemoveDir(t *testing.T) {
	root := t.TempDir()
	fs := dialTestServer(t, root, TLSNone)
	ctx := context.Background()

	if err := fs.Mkdir(ctx, "/newdir"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "newdir")); err != nil {
		t.Fatalf("expected newdir on the real filesystem: %v", err)
	}

	if err := fs.Remove(ctx, "/newdir"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "newdir")); !os.IsNotExist(err) {
		t.Errorf("newdir still exists after Remove: err = %v", err)
	}
}

func TestFTPFS_RemoveAFile(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "gone.txt"), "x")
	fs := dialTestServer(t, root, TLSNone)
	ctx := context.Background()

	if err := fs.Remove(ctx, "/gone.txt"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "gone.txt")); !os.IsNotExist(err) {
		t.Errorf("gone.txt still exists after Remove: err = %v", err)
	}
}

func TestFTPFS_Rename(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "old.txt"), "content")
	fs := dialTestServer(t, root, TLSNone)
	ctx := context.Background()

	if err := fs.Rename(ctx, "/old.txt", "/new.txt"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "new.txt"))
	if err != nil || string(data) != "content" {
		t.Errorf("new.txt = %q, err = %v, want %q", data, err, "content")
	}
}

func TestFTPFS_OpenAndCreateRoundTrip(t *testing.T) {
	root := t.TempDir()
	fs := dialTestServer(t, root, TLSNone)
	ctx := context.Background()

	w, err := fs.Create(ctx, "/written.txt")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := w.Write([]byte("hello ftp")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close (write side): %v", err)
	}

	r, err := fs.Open(ctx, "/written.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "hello ftp" {
		t.Errorf("read back %q, want %q", data, "hello ftp")
	}
}

func TestFTPFS_ExplicitTLS(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "secure.txt"), "over tls")
	fs := dialTestServer(t, root, TLSExplicit)
	ctx := context.Background()

	r, err := fs.Open(ctx, "/secure.txt")
	if err != nil {
		t.Fatalf("Open over explicit TLS: %v", err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "over tls" {
		t.Errorf("read back %q, want %q", data, "over tls")
	}
}

func TestFTPFS_JoinIsAlwaysForwardSlash(t *testing.T) {
	fs := &FTPFS{}
	if got := fs.Join("a", "b", "c"); got != "a/b/c" {
		t.Errorf("Join(a,b,c) = %q, want %q", got, "a/b/c")
	}
}

func TestFTPFS_ParentAtRootHasNoParent(t *testing.T) {
	fs := &FTPFS{}
	if got, ok := fs.Parent("/"); ok || got != "/" {
		t.Errorf("Parent(/) = (%q, %v), want (\"/\", false)", got, ok)
	}
}

func TestFTPFS_ParentOfNestedPath(t *testing.T) {
	fs := &FTPFS{}
	got, ok := fs.Parent("/a/b/c")
	if !ok || got != "/a/b" {
		t.Errorf("Parent(/a/b/c) = (%q, %v), want (\"/a/b\", true)", got, ok)
	}
}
