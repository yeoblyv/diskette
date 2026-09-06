//go:build linux || darwin

package diskspace

import "syscall"

// Query returns total/free bytes for the filesystem containing path, via
// statfs(2). Bsize's underlying type differs between Linux (int64) and
// macOS (uint32); Blocks/Bavail are uint64 on both, so converting
// everything to uint64 before multiplying is what makes one file cover
// both platforms.
func Query(path string) (Usage, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return Usage{}, err
	}
	blockSize := uint64(stat.Bsize)
	return Usage{
		Total: uint64(stat.Blocks) * blockSize,
		Free:  uint64(stat.Bavail) * blockSize,
	}, nil
}
