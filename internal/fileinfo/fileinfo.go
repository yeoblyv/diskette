// Package fileinfo reports filesystem metadata beyond what os.FileInfo
// portably exposes — creation and last-access time, which Go's standard
// library leaves platform-specific since not every OS (or filesystem)
// tracks them. Stat's implementation lives in a per-OS file
// (fileinfo_darwin.go, fileinfo_linux.go, fileinfo_windows.go), the same
// build-tag pattern the rest of this project uses for per-OS code.
package fileinfo

import (
	"os"
	"time"
)

// Info is one entry's metadata for the F1 "file info" view.
type Info struct {
	Name     string
	Size     int64
	IsDir    bool
	Mode     os.FileMode
	Modified time.Time
	Accessed time.Time

	// Created and CreatedKnown report the entry's creation ("birth") time.
	// Not every OS/filesystem combination tracks this — notably Linux only
	// does on newer kernels and filesystems (ext4, btrfs, xfs; not, say,
	// many network filesystems) — so CreatedKnown is false rather than
	// Created being a guess when it can't be determined.
	Created      time.Time
	CreatedKnown bool
}

// Stat reports path's metadata, including platform-specific creation and
// access times that os.Stat's portable os.FileInfo doesn't expose.
func Stat(path string) (Info, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return Info{}, err
	}
	info := Info{
		Name:     fi.Name(),
		Size:     fi.Size(),
		IsDir:    fi.IsDir(),
		Mode:     fi.Mode(),
		Modified: fi.ModTime(),
	}
	populatePlatformTimes(path, fi, &info)
	return info, nil
}
