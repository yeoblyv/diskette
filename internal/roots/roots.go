// Package roots lists the filesystem roots a pane can switch to: drive
// letters on Windows, "/" plus mounted volumes on macOS, "/" plus real
// mount points on Linux. Each OS gets its own file (roots_darwin.go,
// roots_linux.go, roots_windows.go — List's only export), the same
// build-tag pattern graphite itself uses for sys_windows.go vs
// sys_other.go, since the three platforms have nothing in common here
// beyond List's signature: List() []string, returning the roots in a
// stable, sensible order (always starting with "/" or the boot drive) and
// never failing outright — a platform-specific listing error degrades to
// the smallest correct answer instead of surfacing an error a caller has
// no useful way to act on.
package roots
