//go:build linux

package roots

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// skipFSType lists mounted filesystem types that aren't a meaningful
// navigation target for a file manager — pseudo/virtual filesystems the
// kernel exposes for its own bookkeeping, not places a user keeps files.
var skipFSType = map[string]bool{
	"proc": true, "sysfs": true, "cgroup": true, "cgroup2": true,
	"devpts": true, "autofs": true, "debugfs": true, "tracefs": true,
	"securityfs": true, "pstore": true, "bpf": true, "hugetlbfs": true,
	"mqueue": true, "configfs": true, "fusectl": true, "binfmt_misc": true,
	"devtmpfs": true, "sunrpc": true, "rpc_pipefs": true, "tmpfs": true,
}

// List returns "/" plus every other real mount point parsed from
// /proc/self/mountinfo, in the format documented at
// https://man7.org/linux/man-pages/man5/proc.5.html (see the "mountinfo"
// section): the mount point is always field index 4, and a literal "-"
// field separates the variable-length optional fields from the trailing
// filesystem type/source/options, whose first entry (right after the "-")
// is the filesystem type.
func List() []string {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return []string{"/"}
	}
	defer f.Close()
	return parseMountinfo(f)
}

// parseMountinfo does the actual parsing, split out from List so a test
// can feed it a fixed string instead of the real, unmockable
// /proc/self/mountinfo.
func parseMountinfo(r io.Reader) []string {
	seen := map[string]bool{"/": true}
	out := []string{"/"}

	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 5 {
			continue
		}

		sepIdx := -1
		for i, field := range fields {
			if field == "-" {
				sepIdx = i
				break
			}
		}
		if sepIdx < 0 || sepIdx+1 >= len(fields) {
			continue
		}

		mountPoint := fields[4]
		fsType := fields[sepIdx+1]
		if skipFSType[fsType] || seen[mountPoint] {
			continue
		}
		seen[mountPoint] = true
		out = append(out, mountPoint)
	}
	return out
}
