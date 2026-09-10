package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestConfigureAutoSync_BashReturnsRCFileArgsThatSourceTheRealRCFirst(t *testing.T) {
	args := configureAutoSync("bash")
	if len(args) != 3 || args[0] != "--rcfile" || args[2] != "-i" {
		t.Fatalf("configureAutoSync(\"bash\") = %v, want [--rcfile <path> -i]", args)
	}
	t.Cleanup(func() { os.RemoveAll(filepath.Dir(args[1])) })
	data, err := os.ReadFile(args[1])
	if err != nil {
		t.Fatalf("reading rcfile %s: %v", args[1], err)
	}
	content := string(data)
	if !strings.Contains(content, "$HOME/.bashrc") {
		t.Errorf("bash rcfile = %q, want it to source the user's own ~/.bashrc", content)
	}
	if !strings.Contains(content, `eval "$(diskette sync on)"`) {
		t.Errorf("bash rcfile = %q, want it to eval diskette sync on", content)
	}
	if strings.Index(content, "$HOME/.bashrc") > strings.Index(content, "diskette sync on") {
		t.Errorf("bash rcfile = %q, want the user's own rc sourced before the hook is installed", content)
	}
}

func TestConfigureAutoSync_ZshPointsZDOTDIRAtAShimThatRestoresTheRealOne(t *testing.T) {
	t.Setenv("ZDOTDIR", "/original/zdotdir")
	// zshSyncDotDir memoizes on first call — reset it so this test's
	// ZDOTDIR is what gets captured as "the real one" instead of a value
	// some other test already cached.
	zshSyncShim = struct {
		dir string
		err error
		set bool
	}{}
	t.Cleanup(func() {
		if zshSyncShim.dir != "" {
			os.RemoveAll(zshSyncShim.dir)
		}
	})

	if args := configureAutoSync("zsh"); args != nil {
		t.Errorf("configureAutoSync(\"zsh\") args = %v, want nil (it works via ZDOTDIR, not args)", args)
	}
	dir := os.Getenv("ZDOTDIR")
	if dir == "/original/zdotdir" {
		t.Fatal("ZDOTDIR was not redirected to a shim directory")
	}

	data, err := os.ReadFile(filepath.Join(dir, ".zshenv"))
	if err != nil {
		t.Fatalf("reading shim .zshenv: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `export ZDOTDIR="/original/zdotdir"`) {
		t.Errorf(".zshenv = %q, want it to restore the real ZDOTDIR", content)
	}
	if !strings.Contains(content, `eval "$(diskette sync on)"`) {
		t.Errorf(".zshenv = %q, want it to eval diskette sync on", content)
	}
	if strings.Index(content, "export ZDOTDIR") > strings.Index(content, "diskette sync on") {
		t.Errorf(".zshenv = %q, want ZDOTDIR restored before the hook is installed", content)
	}
}

func TestConfigureAutoSync_NilForUnsupportedShellKinds(t *testing.T) {
	for _, kind := range []string{"", "fish", "cmd"} {
		if got := configureAutoSync(kind); got != nil {
			t.Errorf("configureAutoSync(%q) = %v, want nil", kind, got)
		}
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
