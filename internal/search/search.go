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
// cancellation, checked between directories the same way vfs.Walk does.
// A malformed Mask (filepath.Match's ErrBadPattern) is reported as an
// error rather than silently matching nothing.
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

	return vfs.Walk(ctx, fs, root, func(path string, e vfs.Entry) error {
		if path != root { // never match the search root against its own mask
			check(path, e)
		}
		return nil
	})
}
