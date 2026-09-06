// Package diskspace reports total/free bytes for the filesystem containing
// a given path, for the free-space indicator at the bottom of the window.
// Query's implementation lives in a per-OS file (diskspace_unix.go for
// Linux/macOS via syscall.Statfs, diskspace_windows.go via
// GetDiskFreeSpaceEx), the same build-tag pattern graphite itself uses for
// sys_windows.go vs sys_other.go.
package diskspace

// Usage reports total and free bytes on one filesystem.
type Usage struct {
	Total uint64
	Free  uint64
}
