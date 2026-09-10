//go:build windows

package fileinfo

import (
	"os"
	"syscall"
	"time"
)

// populatePlatformTimes fills in Accessed and Created from the raw
// Win32FileAttributeData — Windows always tracks a real creation time,
// unlike Linux/most non-Apple Unix filesystems.
func populatePlatformTimes(path string, fi os.FileInfo, info *Info) {
	stat, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return
	}
	info.Accessed = timeFromFiletime(stat.LastAccessTime)
	info.Created = timeFromFiletime(stat.CreationTime)
	info.CreatedKnown = true
}

func timeFromFiletime(ft syscall.Filetime) time.Time {
	return time.Unix(0, ft.Nanoseconds())
}
