//go:build windows

package diskspace

import "golang.org/x/sys/windows"

// Query returns total/free bytes for the volume containing path, via the
// Win32 GetDiskFreeSpaceEx API.
func Query(path string) (Usage, error) {
	dir, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return Usage{}, err
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(dir, &free, &total, &totalFree); err != nil {
		return Usage{}, err
	}
	return Usage{Total: total, Free: totalFree}, nil
}
