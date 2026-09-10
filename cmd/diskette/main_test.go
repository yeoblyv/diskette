package main

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestCLIShimName_MatchesPlatform(t *testing.T) {
	want := "diskette"
	if runtime.GOOS == "windows" {
		want = "diskette.exe"
	}
	if got := cliShimName(); got != want {
		t.Errorf("cliShimName() = %q, want %q", got, want)
	}
}

// TestCLIShimDir_CreatesAFileLiterallyNamedDiskette guards the actual bug
// that motivated cliShimDir: PATH lookup matches by exact filename, so a
// binary named e.g. "diskette-darwin-arm64" (dist/'s per-platform naming)
// is invisible to a shell typing plain `diskette`, no matter which
// directory it lives in.
func TestCLIShimDir_CreatesAFileLiterallyNamedDiskette(t *testing.T) {
	dir, err := cliShimDir()
	if err != nil {
		t.Fatalf("cliShimDir() error = %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	link := filepath.Join(dir, cliShimName())
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("expected %s to exist: %v", link, err)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	info, err := os.Stat(link)
	if err != nil {
		t.Fatalf("os.Stat(%s) error = %v", link, err)
	}
	wantInfo, err := os.Stat(exe)
	if err != nil {
		t.Fatalf("os.Stat(%s) error = %v", exe, err)
	}
	if !os.SameFile(info, wantInfo) {
		t.Errorf("%s does not resolve to the running executable %s", link, exe)
	}
}
