// Package openwith launches a file in whatever program the host OS has
// associated with it — the same effect as double-clicking it in Finder,
// Explorer, or a desktop file manager — as opposed to F3/F4's $PAGER/
// $EDITOR, which are terminal programs Diskette suspends itself for.
// Open's implementation lives in a per-OS file (openwith_darwin.go,
// openwith_linux.go, openwith_windows.go), the same build-tag pattern
// graphite itself uses for sys_windows.go vs sys_other.go.
package openwith
