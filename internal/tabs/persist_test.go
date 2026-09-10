package tabs

import (
	"os"
	"path/filepath"
	"strings"
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
	for _, k := range []Kind{FileList, Terminal, Remote} {
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

func TestKind_StringNamesEachKind(t *testing.T) {
	cases := map[Kind]string{FileList: "filelist", Terminal: "terminal", Remote: "remote"}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("%v.String() = %q, want %q", k, got, want)
		}
	}
}

func TestKind_UnmarshalRemoteString(t *testing.T) {
	var k Kind
	if err := k.UnmarshalJSON([]byte(`"remote"`)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if k != Remote {
		t.Errorf("k = %v, want Remote", k)
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

func TestSaveLoad_RoundTripsARemoteTabsConnectionMetadataButNeverASecret(t *testing.T) {
	withTempConfigHome(t)

	want := SavedState{
		Left: []SavedTab{
			{
				Kind: Remote, Name: "me@example.com", Pinned: true,
				Remote: &SavedRemote{
					Host: "example.com", Port: 2222, Username: "me",
					AuthMethod: "privatekey", KeyPath: "/home/me/.ssh/id_ed25519",
				},
			},
		},
		LeftActive: 0,
	}

	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(got.Left) != 1 || got.Left[0].Remote == nil {
		t.Fatalf("Left = %+v, want one Remote tab with metadata", got.Left)
	}
	if *got.Left[0].Remote != *want.Left[0].Remote {
		t.Errorf("Remote = %+v, want %+v", got.Left[0].Remote, want.Left[0].Remote)
	}

	// The on-disk file itself must not contain anything secret-shaped —
	// SavedRemote has no password/passphrase field to begin with, but
	// this guards against one ever being added without a second thought.
	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading saved file: %v", err)
	}
	for _, forbidden := range []string{"password", "passphrase", "Password", "Passphrase"} {
		if strings.Contains(string(data), forbidden) {
			t.Errorf("saved tabs.json contains %q — a secret must never be persisted", forbidden)
		}
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
