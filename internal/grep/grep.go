// Package grep implements a grep-style content search: scanning files
// under a root directory line by line for a pattern, reporting each
// matching line's path, line number, and text — as opposed to
// internal/search, which matches file names rather than file contents.
package grep

import (
	"bufio"
	"bytes"
	"context"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yeoblyv/diskette/internal/vfs"
)

// Options configures a Run.
type Options struct {
	// Pattern is the text to search for. Interpreted as a regular
	// expression if Regex is true, otherwise matched literally.
	Pattern string
	// Regex, if true, compiles Pattern as a regexp (RE2 syntax) instead of
	// matching it as a literal substring.
	Regex bool
	// CaseSensitive, if false, matches Pattern regardless of case.
	CaseSensitive bool
	// Mask is a filepath.Match-style glob (e.g. "*.go") restricting which
	// file names are searched; "" behaves like "*" (every file).
	Mask string
	// Recursive, if true, descends into every subdirectory under root;
	// otherwise only root's own immediate files are searched.
	Recursive bool
}

// Match is one hit: a single line of Path that contains Pattern. LineNum
// is 1-based, matching the convention every other line-numbering tool
// (grep included) uses.
type Match struct {
	Path    string
	LineNum int
	Line    string
}

// matcher reports whether a line contains Pattern, per Options.
type matcher func(line string) bool

// Run walks (or, if !opts.Recursive, simply lists) root on fs, opening
// every file whose name matches opts.Mask and calling onMatch for each
// line containing opts.Pattern. It respects ctx cancellation. A malformed
// Mask or Pattern (when Regex is set) is reported as an error immediately;
// a directory or file that turns out to be unreadable partway through is
// skipped instead, the same tradeoff internal/search makes, since one
// inaccessible branch or a permission-denied file shouldn't abort a
// search across the rest of the tree. Files that look binary (a NUL byte
// in the first few KB) are skipped, matching grep's default behavior of
// not dumping binary garbage as "matches".
func Run(ctx context.Context, fs vfs.FileSystem, root string, opts Options, onMatch func(Match)) error {
	match, err := newMatcher(opts)
	if err != nil {
		return err
	}
	mask := opts.Mask
	if mask == "" {
		mask = "*"
	}
	if _, err := filepath.Match(mask, ""); err != nil {
		return err
	}

	nameMatches := func(name string) bool {
		ok, _ := filepath.Match(mask, name)
		return ok
	}

	search := func(path string) {
		searchFile(ctx, fs, path, match, onMatch)
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
			if !e.IsDir && nameMatches(e.Name) {
				search(fs.Join(root, e.Name))
			}
		}
		return nil
	}

	return walk(ctx, fs, root, nameMatches, search)
}

// newMatcher compiles opts into a line-matching function.
func newMatcher(opts Options) (matcher, error) {
	if opts.Regex {
		pattern := opts.Pattern
		if !opts.CaseSensitive {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		return re.MatchString, nil
	}

	pattern := opts.Pattern
	if opts.CaseSensitive {
		return func(line string) bool { return strings.Contains(line, pattern) }, nil
	}
	pattern = strings.ToLower(pattern)
	return func(line string) bool { return strings.Contains(strings.ToLower(line), pattern) }, nil
}

// walk lists dir, searching every matching file and recursing into every
// subdirectory. Only ctx cancellation stops it early.
func walk(ctx context.Context, fs vfs.FileSystem, dir string, nameMatches func(string) bool, search func(path string)) error {
	entries, err := fs.List(ctx, dir)
	if err != nil {
		return nil // can't read this directory; skip it, not fatal to the search
	}
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := fs.Join(dir, e.Name)
		if e.IsDir {
			if err := walk(ctx, fs, path, nameMatches, search); err != nil {
				return err
			}
			continue
		}
		if nameMatches(e.Name) {
			search(path)
		}
	}
	return nil
}

// sniffLimit is how many leading bytes of a file are checked for a NUL
// byte before deciding it's binary and skipping it.
const sniffLimit = 8192

// searchFile opens path and calls onMatch for every line containing
// match's pattern. Any error opening or reading it is treated as "skip
// this file", the same as an unreadable directory during the walk.
func searchFile(ctx context.Context, fs vfs.FileSystem, path string, match matcher, onMatch func(Match)) {
	r, err := fs.Open(ctx, path)
	if err != nil {
		return
	}
	defer r.Close()

	reader := bufio.NewReader(r)
	sniff, _ := reader.Peek(sniffLimit)
	if bytes.IndexByte(sniff, 0) != -1 {
		return
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if ctx.Err() != nil {
			return
		}
		line := scanner.Text()
		if match(line) {
			onMatch(Match{Path: path, LineNum: lineNum, Line: line})
		}
	}
}
