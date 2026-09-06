//go:build windows

package openwith

import "testing"

func TestCommand_UsesCmdStartWithPathArgument(t *testing.T) {
	cmd := command(`C:\Temp\example.txt`)
	want := []string{"cmd", "/c", "start", "", `C:\Temp\example.txt`}
	if len(cmd.Args) != len(want) {
		t.Fatalf("command args = %v, want %v", cmd.Args, want)
	}
	for i, arg := range want {
		if cmd.Args[i] != arg {
			t.Errorf("command args = %v, want %v", cmd.Args, want)
		}
	}
}
