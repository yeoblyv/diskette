//go:build linux

package openwith

import "testing"

func TestCommand_UsesXdgOpenWithPathArgument(t *testing.T) {
	cmd := command("/tmp/example.txt")
	if len(cmd.Args) != 2 || cmd.Args[0] != "xdg-open" || cmd.Args[1] != "/tmp/example.txt" {
		t.Errorf("command args = %v, want [xdg-open /tmp/example.txt]", cmd.Args)
	}
}
