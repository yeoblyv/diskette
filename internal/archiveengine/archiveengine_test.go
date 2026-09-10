package archiveengine

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeoblyv/diskette/internal/vfs"
)

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestDetectFormat(t *testing.T) {
	cases := []struct {
		name string
		want Format
	}{
		{"archive.zip", FormatZip},
		{"ARCHIVE.ZIP", FormatZip},
		{"backup.tar.gz", FormatTarGz},
		{"backup.tgz", FormatTarGz},
		{"backup.tar.bz2", FormatTarBz2},
		{"backup.tbz2", FormatTarBz2},
		{"backup.tbz", FormatTarBz2},
		{"plain.tar", FormatTar},
		{"notes.txt", FormatUnknown},
		{"archive.7z", FormatUnknown},
		{"archive.rar", FormatUnknown},
	}
	for _, tc := range cases {
		if got := DetectFormat(tc.name); got != tc.want {
			t.Errorf("DetectFormat(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCreateZip_RoundTripsFlatFilesAndNestedDirs(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	mustWriteFile(t, filepath.Join(src, "a.txt"), "aaa")
	if err := os.Mkdir(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(src, "sub", "b.txt"), "bbb")

	fs := vfs.LocalFS{}
	archivePath := filepath.Join(dst, "out.zip")
	err := CreateZip(context.Background(), fs,
		[]string{filepath.Join(src, "a.txt"), filepath.Join(src, "sub")},
		fs, archivePath, nil)
	if err != nil {
		t.Fatalf("CreateZip: %v", err)
	}

	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatalf("opening produced zip: %v", err)
	}
	defer zr.Close()

	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{"a.txt", "sub/", "sub/b.txt"} {
		if !names[want] {
			t.Errorf("zip missing entry %q, got %v", want, names)
		}
	}
}

func TestExtractArchive_Zip_RoundTrips(t *testing.T) {
	dst := t.TempDir()
	archivePath := filepath.Join(t.TempDir(), "in.zip")

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	fw, _ := zw.Create("top.txt")
	fw.Write([]byte("top"))
	fw, _ = zw.Create("dir/nested.txt")
	fw.Write([]byte("nested"))
	zw.Close()
	if err := os.WriteFile(archivePath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	fs := vfs.LocalFS{}
	if err := ExtractArchive(context.Background(), fs, archivePath, fs, dst, nil); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}

	if got := mustReadFile(t, filepath.Join(dst, "top.txt")); got != "top" {
		t.Errorf("top.txt = %q, want %q", got, "top")
	}
	if got := mustReadFile(t, filepath.Join(dst, "dir", "nested.txt")); got != "nested" {
		t.Errorf("dir/nested.txt = %q, want %q", got, "nested")
	}
}

func TestExtractArchive_ZipSlipEntryIsClampedNotEscaped(t *testing.T) {
	dst := t.TempDir()
	archivePath := filepath.Join(t.TempDir(), "evil.zip")
	// The naive `filepath.Join(dst, entryName)` a less careful
	// implementation might use resolves "../evil.txt" one level above
	// dst — exactly the escape safeJoin exists to prevent.
	naiveEscapeTarget := filepath.Join(filepath.Dir(dst), "evil.txt")

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	fw, _ := zw.Create("../evil.txt")
	fw.Write([]byte("pwned"))
	zw.Close()
	if err := os.WriteFile(archivePath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	fs := vfs.LocalFS{}
	if err := ExtractArchive(context.Background(), fs, archivePath, fs, dst, nil); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}

	if exists(naiveEscapeTarget) {
		t.Errorf("malicious entry escaped the destination directory to %s", naiveEscapeTarget)
	}
	if !exists(filepath.Join(dst, "evil.txt")) {
		t.Error("expected the clamped entry to land inside dst instead")
	}
}

func TestExtractArchive_PlainTar(t *testing.T) {
	dst := t.TempDir()
	archivePath := filepath.Join(t.TempDir(), "in.tar")

	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	writeTarFile(t, tw, "hello.txt", "hello")
	tw.Close()
	if err := os.WriteFile(archivePath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	fs := vfs.LocalFS{}
	if err := ExtractArchive(context.Background(), fs, archivePath, fs, dst, nil); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}
	if got := mustReadFile(t, filepath.Join(dst, "hello.txt")); got != "hello" {
		t.Errorf("hello.txt = %q, want %q", got, "hello")
	}
}

func TestExtractArchive_TarGz(t *testing.T) {
	dst := t.TempDir()
	archivePath := filepath.Join(t.TempDir(), "in.tar.gz")

	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)
	writeTarFile(t, tw, "compressed.txt", "squeezed")
	tw.Close()
	gw.Close()
	if err := os.WriteFile(archivePath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	fs := vfs.LocalFS{}
	if err := ExtractArchive(context.Background(), fs, archivePath, fs, dst, nil); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}
	if got := mustReadFile(t, filepath.Join(dst, "compressed.txt")); got != "squeezed" {
		t.Errorf("compressed.txt = %q, want %q", got, "squeezed")
	}
}

func TestExtractArchive_UnsupportedFormat(t *testing.T) {
	dst := t.TempDir()
	archivePath := filepath.Join(t.TempDir(), "in.7z")
	mustWriteFile(t, archivePath, "not really 7z")

	fs := vfs.LocalFS{}
	err := ExtractArchive(context.Background(), fs, archivePath, fs, dst, nil)
	if err != ErrUnsupportedFormat {
		t.Errorf("ExtractArchive on .7z = %v, want ErrUnsupportedFormat", err)
	}
}

func writeTarFile(t *testing.T, tw *tar.Writer, name, content string) {
	t.Helper()
	if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(content)), Mode: 0o644}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
}
