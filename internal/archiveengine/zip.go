package archiveengine

import (
	"archive/zip"
	"context"
	"io"
	"strings"

	"github.com/yeoblyv/diskette/internal/vfs"
)

// zipJob is one entry CreateZip will write: srcPath on srcFS, and the
// forward-slash path it gets inside the archive — the zip format's own
// path convention, independent of srcFS's native separator (backslash on
// a Windows LocalFS).
type zipJob struct {
	srcPath  string
	archName string
	entry    vfs.Entry
}

// planZip walks every srcPaths root on fs, pairing each visited entry
// with its archive-internal path: the root's own base name, plus
// whatever lies beneath it for a directory root's descendants — the same
// "name the top-level item, preserve structure under it" convention every
// mainstream archive tool uses for a multi-item selection.
func planZip(ctx context.Context, fs vfs.FileSystem, srcPaths []string) ([]zipJob, error) {
	var jobs []zipJob
	sep := string(pathSeparator(fs))

	for _, root := range srcPaths {
		info, err := fs.Stat(ctx, root)
		if err != nil {
			return nil, err
		}
		rootName := info.Name

		walkErr := vfs.Walk(ctx, fs, root, func(path string, entry vfs.Entry) error {
			archName := rootName
			if path != root {
				rest := strings.TrimPrefix(path, root+sep)
				archName = rootName + "/" + strings.ReplaceAll(rest, sep, "/")
			}
			jobs = append(jobs, zipJob{srcPath: path, archName: archName, entry: entry})
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}
	return jobs, nil
}

// CreateZip packs every entry under srcPaths (on srcFS) into a new .zip
// file at dstPath (on dstFS), streaming each file's contents straight
// from srcFS to the archive writer — nothing is buffered in memory beyond
// zip.Writer's own per-entry deflate state. The full listing is planned
// before dstPath is created, so a source root that happens to live under
// dstFS's own destination directory can never end up including the
// archive it is itself producing.
//
// A source file that fails to open or read is skipped (reported via
// Progress.Err) rather than aborting the whole archive; a failure writing
// to the archive itself (disk full, a broken destination) is not
// recoverable mid-stream and does abort.
func CreateZip(ctx context.Context, srcFS vfs.FileSystem, srcPaths []string, dstFS vfs.FileSystem, dstPath string, onProgress ProgressFunc) error {
	jobs, err := planZip(ctx, srcFS, srcPaths)
	if err != nil {
		return err
	}
	total := len(jobs)
	emit := throttle(onProgress)

	w, err := dstFS.Create(ctx, dstPath)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(w)

	var done int
	for i, j := range jobs {
		if err := ctx.Err(); err != nil {
			zw.Close()
			w.Close()
			return err
		}

		name := j.archName
		method := zip.Deflate
		if j.entry.IsDir {
			name += "/"
			method = zip.Store
		}
		fw, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method, Modified: j.entry.ModTime})
		if err != nil {
			zw.Close()
			w.Close()
			return err
		}

		if !j.entry.IsDir {
			r, openErr := srcFS.Open(ctx, j.srcPath)
			if openErr != nil {
				emit(j.archName, done, total, openErr, i == total-1)
				continue
			}
			_, copyErr := io.Copy(fw, r)
			r.Close()
			if copyErr != nil {
				emit(j.archName, done, total, copyErr, i == total-1)
				continue
			}
		}

		done++
		emit(j.archName, done, total, nil, i == total-1)
	}

	if err := zw.Close(); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}
