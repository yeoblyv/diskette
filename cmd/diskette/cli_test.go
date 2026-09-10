package main

import (
	"strings"
	"testing"
)

func TestShellKindOf(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/bin/zsh", "zsh"},
		{"/usr/local/bin/bash", "bash"},
		{"/opt/homebrew/bin/zsh", "zsh"},
		{"C:\\Windows\\System32\\cmd.exe", ""},
		{"/usr/bin/fish", ""},
		{"/bin/sh", ""},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := shellKindOf(tc.path); got != tc.want {
				t.Errorf("shellKindOf(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestSyncSnippet_OnInstallsAHookThatCallsDisketteView(t *testing.T) {
	for _, kind := range []string{"bash", "zsh"} {
		snippet := syncSnippet(kind, true)
		if !strings.Contains(snippet, `diskette view "$PWD"`) {
			t.Errorf("%s on-snippet = %q, want it to call diskette view \"$PWD\"", kind, snippet)
		}
	}
}

func TestSyncSnippet_OffRemovesTheHook(t *testing.T) {
	bashOff := syncSnippet("bash", false)
	if strings.Contains(bashOff, `diskette view "$PWD"`) {
		t.Errorf("bash off-snippet still installs the hook: %q", bashOff)
	}

	zshOff := syncSnippet("zsh", false)
	if !strings.Contains(zshOff, "add-zsh-hook -d chpwd") {
		t.Errorf("zsh off-snippet = %q, want it to remove the chpwd hook", zshOff)
	}
}

func TestSyncSnippet_BashOnIsIdempotent(t *testing.T) {
	snippet := syncSnippet("bash", true)
	// Running the on-snippet a second time (PROMPT_COMMAND already
	// containing the hook) must not append it twice.
	if strings.Count(snippet, "case") != 1 {
		t.Errorf("bash on-snippet = %q, want exactly one idempotency guard", snippet)
	}
}

func TestLooksLikeShellPath(t *testing.T) {
	if !looksLikeShellPath("/bin/zsh", "zsh") {
		t.Error("looksLikeShellPath(\"/bin/zsh\", \"zsh\") = false, want true")
	}
	if looksLikeShellPath("/bin/bash", "zsh") {
		t.Error("looksLikeShellPath(\"/bin/bash\", \"zsh\") = true, want false")
	}
}
