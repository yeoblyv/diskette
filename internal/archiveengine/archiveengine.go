// Package archiveengine implements the Zip/Unzip gutter buttons' actual
// work — packing a selection into a .zip, and extracting a .zip, .tar,
// .tar.gz/.tgz, or .tar.bz2/.tbz2/.tbz — purely through vfs.FileSystem,
// the same way copyengine implements Copy/Move. UI concerns (progress
// display, error reporting) are pushed out through a callback; this
// package has no dependency on graphite.
package archiveengine

import (
	"context"
	"time"

	"github.com/yeoblyv/diskette/internal/vfs"
)

// Progress reports how far a CreateZip or ExtractArchive call has gotten.
// Total is 0 for a format whose entry count isn't known up front (tar and
// its compressed variants stream sequentially with no central directory
// to count ahead of time) — a caller driving a progress bar should treat
// that as "indeterminate" rather than "already done".
type Progress struct {
	CurrentPath string
	Done, Total int
	// Err is non-nil when CurrentPath failed and was skipped rather than
	// stopping the whole run.
	Err error
}

// ProgressFunc is called as CreateZip/ExtractArchive works, throttled to
// roughly once every 150ms plus a final call for the last item, matching
// copyengine.ProgressFunc's own pacing for a consistent UI story. Like
// that one, it runs on whatever goroutine started the call.
type ProgressFunc func(p Progress)

// pathSeparator returns the byte fs.Join inserts between two non-empty
// elements — copyengine has an identical helper; duplicated here rather
// than shared so the two engines stay independent packages, matching
// copyengine's own "no dependency on anything but vfs" design.
func pathSeparator(fs vfs.FileSystem) byte {
	probe := fs.Join("a", "b")
	if len(probe) == 3 {
		return probe[1]
	}
	return '/'
}

// mkdirAll ensures dir and every missing ancestor exist on fs —
// vfs.FileSystem's own Mkdir requires an existing parent, the same
// constraint os.Mkdir has, so an archive whose directory entries are out
// of order (or missing entirely, common for tar, which need not record
// directories at all) still extracts cleanly. Recursion always bottoms
// out at dstDir, which the caller guarantees already exists.
func mkdirAll(ctx context.Context, fs vfs.FileSystem, dir string) error {
	if _, err := fs.Stat(ctx, dir); err == nil {
		return nil
	}
	if parent, ok := fs.Parent(dir); ok {
		if err := mkdirAll(ctx, fs, parent); err != nil {
			return err
		}
	}
	if err := fs.Mkdir(ctx, dir); err != nil {
		if _, statErr := fs.Stat(ctx, dir); statErr == nil {
			return nil // created by the time we got here; fine
		}
		return err
	}
	return nil
}

// throttle returns an emit function that calls onProgress at most once
// every 150ms, plus always on a forced call (the last item) or one
// carrying an error — the same pacing copyengine.Run uses so a caller
// redrawing a progress bar isn't driven harder than a terminal can
// usefully repaint.
func throttle(onProgress ProgressFunc) func(path string, done, total int, err error, force bool) {
	var lastEmit time.Time
	return func(path string, done, total int, err error, force bool) {
		if onProgress == nil {
			return
		}
		if !force && err == nil && !lastEmit.IsZero() && time.Since(lastEmit) < 150*time.Millisecond {
			return
		}
		onProgress(Progress{CurrentPath: path, Done: done, Total: total, Err: err})
		lastEmit = time.Now()
	}
}
