package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// withCapturedStdout runs fn with os.Stdout redirected to a pipe and
// returns everything written to it — for asserting on cliHelp's output
// without threading an io.Writer through every CLI function just for
// this one test.
func withCapturedStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestRunCLI_BareCommandFallsThroughOutsideDisketteTerminal(t *testing.T) {
	t.Setenv("DISKETTE_IPC", "")
	t.Setenv("DISKETTE_TERMINAL_ID", "")
	if _, handled := runCLI(nil); handled {
		t.Error("runCLI(nil) outside a diskette terminal reported handled=true, want false so main() starts the TUI")
	}
}

func TestRunCLI_UnrecognizedCommandFallsThroughOutsideDisketteTerminal(t *testing.T) {
	t.Setenv("DISKETTE_IPC", "")
	t.Setenv("DISKETTE_TERMINAL_ID", "")
	if _, handled := runCLI([]string{"bogus"}); handled {
		t.Error("runCLI([\"bogus\"]) outside a diskette terminal reported handled=true, want false so main() starts the TUI")
	}
}

func TestRunCLI_BareCommandShowsHelpInsideDisketteTerminal(t *testing.T) {
	t.Setenv("DISKETTE_IPC", "127.0.0.1:0")
	t.Setenv("DISKETTE_TERMINAL_ID", "t1")

	var code int
	var handled bool
	out := withCapturedStdout(t, func() { code, handled = runCLI(nil) })

	if !handled {
		t.Fatal("runCLI(nil) inside a diskette terminal reported handled=false, want true — a bare \"diskette\" here must not fall through to a nested TUI")
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out, "diskette view") || !strings.Contains(out, "diskette sync on|off") {
		t.Errorf("help output = %q, want it to list the available commands", out)
	}
}

func TestRunCLI_UnrecognizedCommandShowsHelpInsideDisketteTerminal(t *testing.T) {
	t.Setenv("DISKETTE_IPC", "127.0.0.1:0")
	t.Setenv("DISKETTE_TERMINAL_ID", "t1")

	var code int
	var handled bool
	out := withCapturedStdout(t, func() { code, handled = runCLI([]string{"bogus"}) })

	if !handled {
		t.Fatal("runCLI([\"bogus\"]) inside a diskette terminal reported handled=false, want true")
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out, "Available commands") {
		t.Errorf("help output = %q, want the command list", out)
	}
}

func TestRunCLI_RecognizedCommandsStillDispatchInsideDisketteTerminal(t *testing.T) {
	t.Setenv("DISKETTE_IPC", "127.0.0.1:0")
	t.Setenv("DISKETTE_TERMINAL_ID", "t1")

	// "select" with the wrong argument count is a recognized command
	// that fails its own validation — it must still take that path
	// (and print its own usage) rather than falling into cliHelp.
	var out string
	code, handled := 0, false
	out = withCapturedStdoutStderr(t, func() { code, handled = runCLI([]string{"select"}) })
	if !handled {
		t.Fatal("runCLI([\"select\"]) reported handled=false, want true")
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "usage: diskette select") {
		t.Errorf("output = %q, want cliSelect's own usage message, not cliHelp's", out)
	}
}

func TestRunCLI_VersionFlagWorksOutsideDisketteTerminal(t *testing.T) {
	t.Setenv("DISKETTE_IPC", "")
	t.Setenv("DISKETTE_TERMINAL_ID", "")

	orig := version
	version = "1.2.3"
	defer func() { version = orig }()

	for _, arg := range []string{"--version", "-v", "version"} {
		var code int
		var handled bool
		out := withCapturedStdout(t, func() { code, handled = runCLI([]string{arg}) })

		if !handled {
			t.Errorf("runCLI([%q]) reported handled=false, want true — --version must work even outside a diskette terminal, unlike view/sync/tag/...", arg)
		}
		if code != 0 {
			t.Errorf("runCLI([%q]) exit code = %d, want 0", arg, code)
		}
		if strings.TrimSpace(out) != "diskette 1.2.3" {
			t.Errorf("runCLI([%q]) output = %q, want %q", arg, out, "diskette 1.2.3\n")
		}
	}
}

func TestCliVersion_PrintsBinaryNameAndVersion(t *testing.T) {
	orig := version
	version = "0.2.0"
	defer func() { version = orig }()

	var code int
	out := withCapturedStdout(t, func() { code = cliVersion() })

	if code != 0 {
		t.Errorf("cliVersion() exit code = %d, want 0", code)
	}
	if strings.TrimSpace(out) != "diskette 0.2.0" {
		t.Errorf("cliVersion() output = %q, want %q", out, "diskette 0.2.0\n")
	}
}

// withCapturedStdoutStderr is withCapturedStdout plus stderr, since
// cliSelect's usage message goes to stderr rather than stdout.
func withCapturedStdoutStderr(t *testing.T, fn func()) string {
	t.Helper()
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wOut, wErr
	defer func() { os.Stdout, os.Stderr = origOut, origErr }()

	fn()

	wOut.Close()
	wErr.Close()
	out, err := io.ReadAll(rOut)
	if err != nil {
		t.Fatal(err)
	}
	errOut, err := io.ReadAll(rErr)
	if err != nil {
		t.Fatal(err)
	}
	return string(out) + string(errOut)
}

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

func TestCliHelp_RespectsDisketteLocale(t *testing.T) {
	t.Setenv("DISKETTE_IPC", "127.0.0.1:0")
	t.Setenv("DISKETTE_TERMINAL_ID", "t1")

	cases := []struct {
		locale string
		want   string // a phrase only that language's translation contains
	}{
		{"", "navigate the other pane"}, // unset falls back to English
		{"en", "navigate the other pane"},
		{"uk", "перейти в іншій панелі"},
		{"ru", "перейти в другой панели"},
		{"nl", "navigeer het andere paneel"},
		{"fr", "navigate the other pane"}, // an unrecognized locale also falls back to English
	}
	for _, tc := range cases {
		t.Run(tc.locale, func(t *testing.T) {
			t.Setenv("DISKETTE_LOCALE", tc.locale)
			out := withCapturedStdout(t, func() { runCLI([]string{"bogus"}) })
			if !strings.Contains(out, tc.want) {
				t.Errorf("DISKETTE_LOCALE=%q: output = %q, want it to contain %q", tc.locale, out, tc.want)
			}
		})
	}
}
