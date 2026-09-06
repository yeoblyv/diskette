//go:build windows

package openwith

import "os/exec"

// command builds (without starting) the process that opens path, split
// out from Open so a test can inspect the command without actually
// launching anything.
func command(path string) *exec.Cmd {
	// "start" is a cmd.exe builtin, not a standalone executable, so it has
	// to be invoked through cmd /c. The empty argument after start is the
	// window title start expects when the target itself might contain
	// spaces or quotes.
	return exec.Command("cmd", "/c", "start", "", path)
}

// Open launches path in whatever application Windows has associated with
// its type — the same mechanism the "start" shell command uses — without
// waiting for it to exit.
func Open(path string) error {
	cmd := command(path)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // reap the child once it exits; we don't care about its result
	return nil
}
