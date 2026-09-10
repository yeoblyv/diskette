package archiveengine

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/yeoblyv/diskette/internal/vfs"
)

// Format identifies a recognized archive format, detected purely from the
// archive's file name — the same way every mainstream file manager
// decides how to open one, without needing to read the file first.
type Format int

// Supported formats. FormatUnknown means DetectFormat didn't recognize
// the name's extension at all.
const (
	FormatUnknown Format = iota
	FormatZip
	FormatTar
	FormatTarGz
	FormatTarBz2
)

// ErrUnsupportedFormat is returned by ExtractArchive when name's
// extension isn't one DetectFormat recognizes — includes formats this
// package deliberately doesn't handle yet, like .7z and .rar, which need
// more than Go's standard library provides (see the package doc).
var ErrUnsupportedFormat = errors.New("archiveengine: unsupported archive format")

// DetectFormat identifies name's archive format from its extension,
// case-insensitively. The two-part ".tar.gz"/".tar.bz2" suffixes are
// checked before the single-part ones so e.g. "backup.tar.gz" isn't
// mistaken for a plain, uncompressed .tar.
func DetectFormat(name string) Format {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return FormatZip
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return FormatTarGz
	case strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tbz2"), strings.HasSuffix(lower, ".tbz"):
		return FormatTarBz2
	case strings.HasSuffix(lower, ".tar"):
		return FormatTar
	default:
		return FormatUnknown
	}
}

// ExtractArchive extracts archivePath (on srcFS) into dstDir (on dstFS,
// which must already exist), auto-detecting the format from archivePath's
// own name. An entry whose path would resolve outside dstDir (a
// maliciously or accidentally crafted archive — the classic "zip slip"
// vulnerability, equally applicable to tar) is clamped back under dstDir
// rather than allowed to escape it; nothing is ever written outside the
// destination directory. An already-existing destination file is
// overwritten without asking, matching plain `tar x`/`7z x`'s own default
// (unlike Copy/Move, there is no per-file conflict prompt here yet).
func ExtractArchive(ctx context.Context, srcFS vfs.FileSystem, archivePath string, dstFS vfs.FileSystem, dstDir string, onProgress ProgressFunc) error {
	switch DetectFormat(archivePath) {
	case FormatZip:
		return extractZip(ctx, srcFS, archivePath, dstFS, dstDir, onProgress)
	case FormatTar:
		return extractTar(ctx, srcFS, archivePath, dstFS, dstDir, nil, onProgress)
	case FormatTarGz:
		return extractTar(ctx, srcFS, archivePath, dstFS, dstDir, func(r io.Reader) (io.Reader, error) {
			return gzip.NewReader(r)
		}, onProgress)
	case FormatTarBz2:
		return extractTar(ctx, srcFS, archivePath, dstFS, dstDir, func(r io.Reader) (io.Reader, error) {
			return bzip2.NewReader(r), nil
		}, onProgress)
	default:
		return ErrUnsupportedFormat
	}
}

// safeJoin resolves name — an archive entry's own path, always
// forward-slash per both the zip and tar formats regardless of host OS —
// against dstDir on dstFS. Any ".." component or leading "/" that would
// otherwise place the result outside dstDir is clamped back under it
// (path.Clean against a synthetic root, the standard zip-slip mitigation)
// rather than followed, so a hostile or corrupt archive can never write
// beyond the destination directory. Returns false only for a name that
// cleans away to nothing.
func safeJoin(dstFS vfs.FileSystem, dstDir, name string) (string, bool) {
	name = strings.ReplaceAll(name, "\\", "/")
	clean := strings.TrimPrefix(path.Clean("/"+name), "/")
	if clean == "" || clean == "." {
		return "", false
	}
	parts := strings.Split(clean, "/")
	joinArgs := append([]string{dstDir}, parts...)
	return dstFS.Join(joinArgs...), true
}

// extractZip implements the .zip case. The whole file is read into memory
// first: archive/zip.NewReader needs io.ReaderAt plus the file's size to
// locate the central directory at the end, which a plain streaming
// vfs.FileSystem.Open can't offer — acceptable for the archive sizes a
// file manager's Unzip button actually sees, and it keeps this working
// over any vfs.FileSystem (a future SFTPFS included), not just a local
// path os.Open could seek directly.
func extractZip(ctx context.Context, srcFS vfs.FileSystem, archivePath string, dstFS vfs.FileSystem, dstDir string, onProgress ProgressFunc) error {
	rc, err := srcFS.Open(ctx, archivePath)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return err
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}

	total := len(zr.File)
	emit := throttle(onProgress)
	var done int

	for i, f := range zr.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		force := i == total-1

		dstPath, ok := safeJoin(dstFS, dstDir, f.Name)
		if !ok {
			emit(f.Name, done, total, fmt.Errorf("skipping empty entry name"), force)
			continue
		}

		if f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/") {
			if err := mkdirAll(ctx, dstFS, dstPath); err != nil {
				emit(f.Name, done, total, err, force)
				continue
			}
			done++
			emit(f.Name, done, total, nil, force)
			continue
		}

		if parent, ok := dstFS.Parent(dstPath); ok {
			if err := mkdirAll(ctx, dstFS, parent); err != nil {
				emit(f.Name, done, total, err, force)
				continue
			}
		}

		if extractErr := extractZipEntry(ctx, f, dstFS, dstPath); extractErr != nil {
			emit(f.Name, done, total, extractErr, force)
			continue
		}
		done++
		emit(f.Name, done, total, nil, force)
	}
	return nil
}

func extractZipEntry(ctx context.Context, f *zip.File, dstFS vfs.FileSystem, dstPath string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	w, err := dstFS.Create(ctx, dstPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, rc); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

// extractTar implements the .tar case and, via decompress, its gzip/bzip2
// variants — tar streams its entries sequentially with no central
// directory, so unlike extractZip this never needs random access and
// never buffers the whole archive in memory, but also can't know Total
// ahead of time (Progress.Total is always 0 here).
func extractTar(ctx context.Context, srcFS vfs.FileSystem, archivePath string, dstFS vfs.FileSystem, dstDir string, decompress func(io.Reader) (io.Reader, error), onProgress ProgressFunc) error {
	rc, err := srcFS.Open(ctx, archivePath)
	if err != nil {
		return err
	}
	defer rc.Close()

	var r io.Reader = rc
	if decompress != nil {
		dr, err := decompress(rc)
		if err != nil {
			return err
		}
		r = dr
	}

	tr := tar.NewReader(r)
	emit := throttle(onProgress)
	var done int

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		hdr, err := tr.Next()
		if err == io.EOF {
			emit("", done, 0, nil, true)
			return nil
		}
		if err != nil {
			return err
		}

		dstPath, ok := safeJoin(dstFS, dstDir, hdr.Name)
		if !ok {
			emit(hdr.Name, done, 0, fmt.Errorf("skipping empty entry name"), false)
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := mkdirAll(ctx, dstFS, dstPath); err != nil {
				emit(hdr.Name, done, 0, err, false)
				continue
			}
		case tar.TypeReg:
			if parent, ok := dstFS.Parent(dstPath); ok {
				if err := mkdirAll(ctx, dstFS, parent); err != nil {
					emit(hdr.Name, done, 0, err, false)
					continue
				}
			}
			w, err := dstFS.Create(ctx, dstPath)
			if err != nil {
				emit(hdr.Name, done, 0, err, false)
				continue
			}
			if _, err := io.Copy(w, tr); err != nil {
				w.Close()
				emit(hdr.Name, done, 0, err, false)
				continue
			}
			if err := w.Close(); err != nil {
				emit(hdr.Name, done, 0, err, false)
				continue
			}
		default:
			// Symlinks, hard links, devices, etc.: vfs.FileSystem has no
			// way to represent any of these, so they're skipped rather
			// than failing the whole archive over one entry most
			// extractions don't actually need.
			emit(hdr.Name, done, 0, nil, false)
			continue
		}

		done++
		emit(hdr.Name, done, 0, nil, false)
	}
}
