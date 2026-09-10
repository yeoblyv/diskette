//go:build darwin

package fileinfo

import (
	"os"
	"syscall"
	"time"
)

// populatePlatformTimes fills in Accessed/Created from the raw stat
// result — macOS's Stat_t always carries a real birth time.
func populatePlatformTimes(path string, fi os.FileInfo, info *Info) {
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	info.Accessed = time.Unix(stat.Atimespec.Sec, stat.Atimespec.Nsec)
	info.Created = time.Unix(stat.Birthtimespec.Sec, stat.Birthtimespec.Nsec)
	info.CreatedKnown = true
}
