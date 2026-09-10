package tabs

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// SavedTab is a pinned tab's on-disk representation — no live Widget,
// since a running shell or an open directory listing isn't something to
// serialize. Path only means anything for a FileList tab (letting it
// reopen at the same directory); it's ignored for Terminal.
type SavedTab struct {
	Kind   Kind   `json:"kind"`
	Name   string `json:"name"`
	Path   string `json:"path,omitempty"`
	Pinned bool   `json:"pinned"`
}

// MarshalJSON encodes Kind as its String() form ("filelist"/"terminal")
// rather than a bare int, so the saved file stays readable (and stable
// across a future reordering of the Kind constants) if a user or another
// tool ever opens it.
func (k Kind) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

// UnmarshalJSON is MarshalJSON's inverse; an unrecognized string decodes
// as FileList rather than failing the whole load over one bad tab.
func (k *Kind) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s == "terminal" {
		*k = Terminal
	} else {
		*k = FileList
	}
	return nil
}

// SavedState is the whole on-disk file: each pane's pinned tabs, in
// order, with the pane's active one marked by index — -1 if none of the
// saved tabs was the active one (e.g. the active tab wasn't pinned).
type SavedState struct {
	Left        []SavedTab `json:"left"`
	LeftActive  int        `json:"leftActive"`
	Right       []SavedTab `json:"right"`
	RightActive int        `json:"rightActive"`
}

// configPath returns where pinned tabs are saved: a per-user config
// directory (via os.UserConfigDir, so this is already correct per OS —
// e.g. ~/Library/Application Support on macOS, %AppData% on Windows,
// $XDG_CONFIG_HOME or ~/.config on Linux) rather than anything
// diskette-specific to each platform.
func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "diskette", "tabs.json"), nil
}

// Load reads the saved pinned-tab state. A missing file (nothing pinned
// yet, or a fresh install) is reported the same as any other read error —
// the caller falls back to its own default (see cmd/diskette/main.go) —
// rather than treated as a special "empty but valid" case.
func Load() (SavedState, error) {
	path, err := configPath()
	if err != nil {
		return SavedState{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return SavedState{}, err
	}
	var state SavedState
	if err := json.Unmarshal(data, &state); err != nil {
		return SavedState{}, err
	}
	return state, nil
}

// Save writes state, creating the config directory if it doesn't exist
// yet.
func Save(state SavedState) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
