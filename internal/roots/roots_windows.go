//go:build windows

package roots

import "golang.org/x/sys/windows"

// List returns every currently mounted drive letter (e.g. "C:\", "D:\"),
// queried via the Win32 logical-drives bitmask (bit 0 = A, bit 1 = B, ...).
func List() []string {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return []string{`C:\`}
	}
	return drivesFromMask(mask)
}

// drivesFromMask decodes a Win32 logical-drives bitmask (bit 0 = A, bit 1
// = B, ...) into drive-letter paths, split out from List so a test can
// exercise the decoding without the real, platform-only syscall.
func drivesFromMask(mask uint32) []string {
	var out []string
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) != 0 {
			out = append(out, string(rune('A'+i))+`:\`)
		}
	}
	return out
}
