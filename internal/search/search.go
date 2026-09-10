// Package search implements F3's "find files" query: matching entries by
// name mask under a root directory, optionally recursing into
// subdirectories.
package search

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/yeoblyv/diskette/internal/vfs"
)

// Options configures a Run.
type Options struct {
	// Mask is a filepath.Match-style glob (e.g. "*.go") matched against
	// each entry's name — not its full path.
	Mask string
	// Recursive, if true, descends into every subdirectory under root;
	// otherwise only root's own immediate entries are checked.
	Recursive bool
	// CaseSensitive, if false, folds both Mask and each entry's name to
	// lowercase before matching.
	CaseSensitive bool
}

// Match is one hit: path is its full path, joined via fs.Join the same
// way the rest of the program builds one.
type Match struct {
	Path  string
	Entry vfs.Entry
}

// Run walks (or, if !opts.Recursive, simply lists) root on fs, calling
// onMatch for every entry whose name matches opts.Mask. It respects ctx
// cancellation. A malformed Mask (filepath.Match's ErrBadPattern) is
// reported as an error rather than silently matching nothing; root itself
// being unreadable is too, since the user named it explicitly. A
// subdirectory encountered while recursing that turns out to be
// unreadable (permission denied — macOS's own ~/.Trash is a common one —
// a broken symlink, ...) is skipped instead: one inaccessible branch
// shouldn't abort a search across the rest of the tree.
func Run(ctx context.Context, fs vfs.FileSystem, root string, opts Options, onMatch func(Match)) error {
	mask := opts.Mask
	if mask == "" {
		mask = "*"
	}
	if !opts.CaseSensitive {
		mask = strings.ToLower(mask)
	}
	// Validate the pattern once up front — filepath.Match re-parses it on
	// every call otherwise, and a bad pattern should fail the whole search
	// immediately rather than simply never matching.
	if _, err := filepath.Match(mask, ""); err != nil {
		return err
	}

	matches := func(name string) bool {
		if !opts.CaseSensitive {
			name = strings.ToLower(name)
		}
		ok, _ := filepath.Match(mask, name)
		return ok
	}

	check := func(path string, e vfs.Entry) {
		if matches(e.Name) {
			onMatch(Match{Path: path, Entry: e})
		}
	}

	if !opts.Recursive {
		entries, err := fs.List(ctx, root)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			check(fs.Join(root, e.Name), e)
		}
		return nil
	}

	return walk(ctx, fs, root, check)
}

// walk lists dir, calling check for every entry and recursing into every
// subdirectory — deliberately not vfs.Walk, whose Stat-then-List on each
// directory propagates any single directory's error as fatal to the
// entire traversal, which is the wrong tradeoff for a "search my whole
// home directory" query where an inaccessible directory or two is
// unremarkable. Only ctx cancellation stops it early.
func walk(ctx context.Context, fs vfs.FileSystem, dir string, check func(path string, e vfs.Entry)) error {
	entries, err := fs.List(ctx, dir)
	if err != nil {
		return nil // can't read this directory; skip it, not fatal to the search
	}
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := fs.Join(dir, e.Name)
		check(path, e)
		if e.IsDir {
			if err := walk(ctx, fs, path, check); err != nil {
				return err
			}
		}
	}
	return nil
}
