package main

import (
	"path/filepath"
	"testing"
)

func TestPathWithSelfDir_PrependsWhenAbsent(t *testing.T) {
	sep := string(filepath.ListSeparator)
	got := pathWithSelfDir("/usr/bin"+sep+"/bin", "/opt/diskette")
	want := "/opt/diskette" + sep + "/usr/bin" + sep + "/bin"
	if got != want {
		t.Errorf("pathWithSelfDir = %q, want %q", got, want)
	}
}

func TestPathWithSelfDir_LeavesPathAloneWhenAlreadyPresent(t *testing.T) {
	sep := string(filepath.ListSeparator)
	path := "/usr/bin" + sep + "/opt/diskette" + sep + "/bin"
	if got := pathWithSelfDir(path, "/opt/diskette"); got != path {
		t.Errorf("pathWithSelfDir = %q, want unchanged %q", got, path)
	}
}

func TestPathWithSelfDir_HandlesEmptyPath(t *testing.T) {
	if got := pathWithSelfDir("", "/opt/diskette"); got != "/opt/diskette" {
		t.Errorf("pathWithSelfDir(\"\", ...) = %q, want %q", got, "/opt/diskette")
	}
}
