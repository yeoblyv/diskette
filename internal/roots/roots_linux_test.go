//go:build linux

package roots

import (
	"strings"
	"testing"
)

// A real (trimmed) /proc/self/mountinfo excerpt: root filesystem, a
// pseudo-filesystem that must be skipped, and a real external disk mount
// that must be kept.
const sampleMountinfo = `22 1 259:2 / / rw,relatime shared:1 - ext4 /dev/root rw
23 22 0:20 / /proc rw,nosuid - proc proc rw
45 22 259:3 / /media/user/EXTERNAL rw,relatime shared:3 - vfat /dev/sdb1 rw,uid=1000
46 22 0:25 / /run rw,nosuid - tmpfs tmpfs rw,size=100000k
`

func TestParseMountinfo_KeepsRealMountsSkipsPseudo(t *testing.T) {
	got := parseMountinfo(strings.NewReader(sampleMountinfo))

	want := map[string]bool{"/": true, "/media/user/EXTERNAL": true}
	if len(got) != len(want) {
		t.Fatalf("parseMountinfo() = %v, want exactly %v", got, want)
	}
	for _, g := range got {
		if !want[g] {
			t.Errorf("parseMountinfo() included %q, want it skipped (pseudo-fs or duplicate)", g)
		}
	}
}

func TestParseMountinfo_AlwaysIncludesRootEvenIfAbsent(t *testing.T) {
	got := parseMountinfo(strings.NewReader(""))
	if len(got) != 1 || got[0] != "/" {
		t.Errorf("parseMountinfo(empty) = %v, want [\"/\"]", got)
	}
}

func TestParseMountinfo_MalformedLinesAreSkippedNotFatal(t *testing.T) {
	got := parseMountinfo(strings.NewReader("garbage line with too few fields\n" + sampleMountinfo))
	found := false
	for _, g := range got {
		if g == "/media/user/EXTERNAL" {
			found = true
		}
	}
	if !found {
		t.Errorf("a malformed leading line broke parsing of valid lines after it: %v", got)
	}
}
