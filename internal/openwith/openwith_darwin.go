//go:build darwin

package openwith

import "os/exec"

// command builds (without starting) the process that opens path, split
// out from Open so a test can inspect the command without actually
// launching anything.
func command(path string) *exec.Cmd {
	return exec.Command("open", path)
}

// Open launches path in whatever application macOS has associated with
// its type — the same "open" command Finder itself uses — without
// waiting for it to exit.
func Open(path string) error {
	cmd := command(path)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // reap the child once it exits; we don't care about its result
	return nil
}
