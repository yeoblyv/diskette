package tabs

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempConfigHome points os.UserConfigDir (via XDG_CONFIG_HOME, which
// takes priority on every OS Go's implementation checks first on Unix;
// see os.UserConfigDir's own docs) at a fresh temp directory, so Load/Save
// tests never touch the real user's config.
func withTempConfigHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	switch {
	case os.Getenv("APPDATA") != "" || isWindows():
		t.Setenv("APPDATA", dir)
	default:
		t.Setenv("XDG_CONFIG_HOME", dir)
		t.Setenv("HOME", dir) // macOS's UserConfigDir ignores XDG_CONFIG_HOME and uses $HOME/Library/Application Support
	}
}

func TestSaveLoad_RoundTrips(t *testing.T) {
	withTempConfigHome(t)

	want := SavedState{
		Left: []SavedTab{
			{Kind: FileList, Name: "Projects", Path: "/home/me/projects", Pinned: true},
		},
		LeftActive: 0,
		Right: []SavedTab{
			{Kind: Terminal, Name: "Shell", Pinned: true},
		},
		RightActive: 0,
	}

	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(got.Left) != 1 || got.Left[0] != want.Left[0] {
		t.Errorf("Left = %+v, want %+v", got.Left, want.Left)
	}
	if len(got.Right) != 1 || got.Right[0] != want.Right[0] {
		t.Errorf("Right = %+v, want %+v", got.Right, want.Right)
	}
}

func TestLoad_MissingFileReturnsAnError(t *testing.T) {
	withTempConfigHome(t)

	if _, err := Load(); err == nil {
		t.Error("Load() with no saved file returned no error, want one (the caller falls back to its own default)")
	}
}

func TestKind_JSONRoundTrip(t *testing.T) {
	for _, k := range []Kind{FileList, Terminal} {
		data, err := k.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON(%v): %v", k, err)
		}
		var got Kind
		if err := got.UnmarshalJSON(data); err != nil {
			t.Fatalf("UnmarshalJSON(%s): %v", data, err)
		}
		if got != k {
			t.Errorf("round-tripped %v as %q, got back %v", k, data, got)
		}
	}
}

func TestKind_UnmarshalUnrecognizedStringFallsBackToFileList(t *testing.T) {
	var k Kind
	if err := k.UnmarshalJSON([]byte(`"something-new"`)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if k != FileList {
		t.Errorf("k = %v, want FileList as the fallback for an unrecognized kind", k)
	}
}

func TestSave_CreatesTheConfigDirectory(t *testing.T) {
	withTempConfigHome(t)

	if err := Save(SavedState{}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Errorf("config directory not created: %v", err)
	}
}

func isWindows() bool { return os.PathSeparator == '\\' }
