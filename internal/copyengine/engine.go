// Package copyengine implements Copy/Move as a single algorithm operating
// purely through vfs.FileSystem, so it never knows whether either side is
// the local disk or a remote server. UI concerns — progress display and
// conflict prompts — are pushed out through callbacks the caller supplies;
// this package has no dependency on graphite.
package copyengine

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/yeoblyv/diskette/internal/vfs"
)

// ConflictAction is the caller's decision for how to resolve one
// destination path that already exists.
type ConflictAction int

// Supported conflict resolutions.
const (
	Overwrite ConflictAction = iota
	Skip
	Rename
	Cancel
)

// Conflict describes one destination path that already exists.
type Conflict struct {
	// Path is the colliding destination path.
	Path string
	// IsDir reports whether the colliding destination entry is itself a
	// directory (relevant because Rename on a directory only renames that
	// directory, not the semantics of merging its contents).
	IsDir bool
}

// Resolution is the caller's answer to a Conflict.
type Resolution struct {
	Action ConflictAction
	// NewName is used only when Action is Rename: the new base name for
	// the colliding destination entry, in its existing parent directory.
	NewName string
	// ForAll applies this Resolution to every later conflict in the same
	// Run call too, without asking again.
	ForAll bool
}

// ResolveFunc is called synchronously, from whatever goroutine Run
// executes on, when a destination path already exists. It must return
// without blocking on anything other than obtaining the answer itself —
// a UI-backed implementation typically hands off to the main loop (e.g.
// graphite's Application.Invoke) and blocks on a channel for the
// resulting Resolution.
type ResolveFunc func(c Conflict) Resolution

// Progress reports how far a Run call has gotten.
type Progress struct {
	CurrentPath string
	FilesDone   int
	FilesTotal  int
	// Err is non-nil when CurrentPath failed and was skipped rather than
	// stopping the whole run.
	Err error
}

// ProgressFunc is called as Run works, throttled to roughly once every
// 150ms plus a final call for the last item, so a caller redrawing a
// progress bar isn't driven harder than a terminal can usefully repaint.
// Like ResolveFunc, it runs on whatever goroutine Run executes on.
type ProgressFunc func(p Progress)

// Task describes one copy or move to perform.
type Task struct {
	SrcFS    vfs.FileSystem
	DstFS    vfs.FileSystem
	SrcPaths []string
	DstDir   string
	// Move removes each source item once every one of its descendants has
	// been copied successfully. A source subtree that had any Skip
	// resolution beneath it is left in place rather than partially
	// deleted.
	Move bool
}

// ErrCanceledByUser is returned by Run when a ResolveFunc answers Cancel.
var ErrCanceledByUser = errors.New("copyengine: canceled by user")

// job is one planned filesystem operation: create dstPath (a directory) or
// copy srcPath's contents to it (a file).
type job struct {
	srcPath string
	dstPath string
	entry   vfs.Entry
	rootIdx int
}

// plan recursively walks every task.SrcPaths root on task.SrcFS, pairing
// each visited entry with its destination path on task.DstFS. Destination
// paths are always built via task.DstFS.Join, never by manipulating
// source-path strings, so SrcFS and DstFS may use different path
// conventions (OS-native vs POSIX).
func plan(ctx context.Context, task Task) ([]job, error) {
	var jobs []job
	var walk func(srcPath, dstPath string, rootIdx int) error
	walk = func(srcPath, dstPath string, rootIdx int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := task.SrcFS.Stat(ctx, srcPath)
		if err != nil {
			return err
		}
		jobs = append(jobs, job{srcPath: srcPath, dstPath: dstPath, entry: info, rootIdx: rootIdx})
		if !info.IsDir {
			return nil
		}
		children, err := task.SrcFS.List(ctx, srcPath)
		if err != nil {
			return err
		}
		for _, c := range children {
			if err := walk(task.SrcFS.Join(srcPath, c.Name), task.DstFS.Join(dstPath, c.Name), rootIdx); err != nil {
				return err
			}
		}
		return nil
	}

	for i, srcPath := range task.SrcPaths {
		info, err := task.SrcFS.Stat(ctx, srcPath)
		if err != nil {
			return nil, err
		}
		if err := walk(srcPath, task.DstFS.Join(task.DstDir, info.Name), i); err != nil {
			return nil, err
		}
	}
	return jobs, nil
}

// pathSeparator returns the byte fs.Join inserts between two non-empty
// elements, so prefix rewriting after a Rename resolution can tell a real
// path-boundary match from a name that merely starts with the same
// characters (e.g. "photos" vs "photos-old").
func pathSeparator(fs vfs.FileSystem) byte {
	probe := fs.Join("a", "b")
	if len(probe) == 3 {
		return probe[1]
	}
	return '/'
}

// Run performs task, calling onProgress as it works and onConflict for
// each destination path that already exists, until every planned entry is
// processed, ctx is canceled, or a conflict is answered Cancel. It returns
// how many entries were actually created/copied and the error that ended
// the run early, or nil for a run that reached the end (per-item errors
// recorded via Progress.Err are not fatal and do not appear here).
func Run(ctx context.Context, task Task, onProgress ProgressFunc, onConflict ResolveFunc) (int, error) {
	jobs, err := plan(ctx, task)
	if err != nil {
		return 0, err
	}
	total := len(jobs)
	sep := pathSeparator(task.DstFS)

	var (
		done       int
		lastEmit   time.Time
		forAll     *Resolution
		skipDirs   []string // dst paths whose whole subtree is being skipped
		renameFrom []string // old dst-path prefixes rewritten by a Rename resolution
		renameTo   []string // parallel to renameFrom
	)

	emit := func(path string, jobErr error, force bool) {
		if onProgress == nil {
			return
		}
		if !force && !lastEmit.IsZero() && time.Since(lastEmit) < 150*time.Millisecond {
			return
		}
		onProgress(Progress{CurrentPath: path, FilesDone: done, FilesTotal: total, Err: jobErr})
		lastEmit = time.Now()
	}

	underPrefix := func(path, prefix string) bool {
		return path == prefix || (strings.HasPrefix(path, prefix) && path[len(prefix)] == sep)
	}

	applyRenames := func(path string) string {
		for i, from := range renameFrom {
			if underPrefix(path, from) {
				return renameTo[i] + path[len(from):]
			}
		}
		return path
	}

	isSkipped := func(path string) bool {
		for _, d := range skipDirs {
			if underPrefix(path, d) {
				return true
			}
		}
		return false
	}

	rootSkipped := make(map[int]bool)

	for i, j := range jobs {
		if err := ctx.Err(); err != nil {
			return done, err
		}

		dstPath := applyRenames(j.dstPath)
		if isSkipped(dstPath) {
			continue
		}

		resolution := Resolution{Action: Overwrite}
		if _, statErr := task.DstFS.Stat(ctx, dstPath); statErr == nil {
			switch {
			case forAll != nil:
				resolution = *forAll
			case onConflict != nil:
				resolution = onConflict(Conflict{Path: dstPath, IsDir: j.entry.IsDir})
				if resolution.ForAll {
					r := resolution
					forAll = &r
				}
			}

			switch resolution.Action {
			case Cancel:
				return done, ErrCanceledByUser
			case Skip:
				rootSkipped[j.rootIdx] = true
				if j.entry.IsDir {
					skipDirs = append(skipDirs, dstPath)
				}
				continue
			case Rename:
				parent, _ := task.DstFS.Parent(dstPath)
				renamed := task.DstFS.Join(parent, resolution.NewName)
				renameFrom = append(renameFrom, dstPath)
				renameTo = append(renameTo, renamed)
				dstPath = renamed
			}
		}

		var opErr error
		if j.entry.IsDir {
			if _, statErr := task.DstFS.Stat(ctx, dstPath); statErr != nil {
				opErr = task.DstFS.Mkdir(ctx, dstPath)
			}
		} else {
			opErr = copyFile(ctx, task.SrcFS, j.srcPath, task.DstFS, dstPath)
		}

		if opErr != nil {
			rootSkipped[j.rootIdx] = true
			emit(dstPath, opErr, i == len(jobs)-1)
			continue
		}

		done++
		emit(dstPath, nil, i == len(jobs)-1)

		if task.Move && !j.entry.IsDir {
			if err := task.SrcFS.Remove(ctx, j.srcPath); err != nil {
				rootSkipped[j.rootIdx] = true
			}
		}
	}

	if task.Move {
		removeMovedSources(ctx, task, rootSkipped)
	}

	return done, nil
}

// copyFile streams src's contents to dst, opening the destination only
// after the source is confirmed readable so a missing/unreadable source
// never leaves behind a truncated destination file.
func copyFile(ctx context.Context, srcFS vfs.FileSystem, srcPath string, dstFS vfs.FileSystem, dstPath string) error {
	r, err := srcFS.Open(ctx, srcPath)
	if err != nil {
		return err
	}
	defer r.Close()

	w, err := dstFS.Create(ctx, dstPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, r); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

// removeMovedSources deletes each top-level source that moved cleanly (no
// skip and no per-file error recorded against its rootIdx).
func removeMovedSources(ctx context.Context, task Task, rootSkipped map[int]bool) {
	for i, srcPath := range task.SrcPaths {
		if rootSkipped[i] {
			continue
		}
		RemoveAll(ctx, task.SrcFS, srcPath)
	}
}

// RemoveAll deletes path and, if it is a directory, everything beneath it —
// the F8 Delete counterpart to Run's Copy/Move, exposed as its own
// function since deleting isn't itself a copy. It walks bottom-up so a
// directory is only removed once every entry beneath it is already gone.
func RemoveAll(ctx context.Context, fs vfs.FileSystem, path string) error {
	var toRemove []string
	if err := vfs.Walk(ctx, fs, path, func(p string, _ vfs.Entry) error {
		toRemove = append(toRemove, p)
		return nil
	}); err != nil {
		return err
	}
	for i := len(toRemove) - 1; i >= 0; i-- {
		if err := fs.Remove(ctx, toRemove[i]); err != nil {
			return err
		}
	}
	return nil
}
