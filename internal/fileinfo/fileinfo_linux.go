//go:build linux

package fileinfo

import (
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// populatePlatformTimes fills in Accessed always (every Linux filesystem
// tracks atime), and Created only when statx actually reports it — btime
// is a newer addition (kernel 4.11+) that not every filesystem populates,
// so CreatedKnown reflects statx's own STATX_BTIME mask bit rather than
// assuming support.
func populatePlatformTimes(path string, fi os.FileInfo, info *Info) {
	if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
		info.Accessed = time.Unix(stat.Atim.Sec, stat.Atim.Nsec)
	}

	var stx unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, path, 0, unix.STATX_BTIME, &stx); err != nil {
		return
	}
	if stx.Mask&unix.STATX_BTIME != 0 {
		info.Created = time.Unix(stx.Btime.Sec, int64(stx.Btime.Nsec))
		info.CreatedKnown = true
	}
}
