package vfs

import "context"

// WalkFunc is called once per entry visited by Walk, with its full path and
// Entry. Returning an error stops the walk; that error is Walk's own
// return value.
type WalkFunc func(path string, entry Entry) error

// Walk recursively visits root and everything beneath it on fs, in
// pre-order (a directory is visited before its children), so a copy can
// create the destination directory before writing into it. It respects
// ctx cancellation between directories.
func Walk(ctx context.Context, fs FileSystem, root string, fn WalkFunc) error {
	info, err := fs.Stat(ctx, root)
	if err != nil {
		return err
	}
	if err := fn(root, info); err != nil {
		return err
	}
	if !info.IsDir {
		return nil
	}

	entries, err := fs.List(ctx, root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := Walk(ctx, fs, fs.Join(root, e.Name), fn); err != nil {
			return err
		}
	}
	return nil
}
