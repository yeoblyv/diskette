// Package pathutil provides small, cross-platform path helpers shared by
// the file panes and the copy engine.
package pathutil

import "path/filepath"

// Parent returns the cleaned parent directory of path and true, or path
// itself (cleaned) and false when path is already a filesystem root (e.g.
// "/" on Unix or "C:\" on Windows) with no parent to navigate to.
func Parent(path string) (string, bool) {
	clean := filepath.Clean(path)
	parent := filepath.Dir(clean)
	if parent == clean {
		return clean, false
	}
	return parent, true
}

// IsRoot reports whether path is a filesystem root with no parent
// directory to navigate to.
func IsRoot(path string) bool {
	_, ok := Parent(path)
	return !ok
}
