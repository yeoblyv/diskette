//go:build darwin

package roots

import "os"

// List returns "/" plus every entry under /Volumes — macOS's convention
// for mounted external disks, disk images, and network shares.
func List() []string {
	return append([]string{"/"}, volumesUnder("/Volumes")...)
}

// volumesUnder lists the mounted-volume directories under dir, split out
// from List so a test can point it at a temporary directory instead of
// the real, unmockable /Volumes.
func volumesUnder(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, dir+"/"+e.Name())
		}
	}
	return out
}
