//go:build darwin

package openwith

import "testing"

func TestCommand_UsesOpenWithPathArgument(t *testing.T) {
	cmd := command("/tmp/example.txt")
	if len(cmd.Args) != 2 || cmd.Args[0] != "open" || cmd.Args[1] != "/tmp/example.txt" {
		t.Errorf("command args = %v, want [open /tmp/example.txt]", cmd.Args)
	}
}
