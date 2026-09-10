package sftpfs

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

// testKey generates a fresh ed25519 ssh.PublicKey, distinct on every call —
// used to simulate "the server offered this key" without any real network
// connection or real host key.
func testKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// fakeAddr is a minimal net.Addr for exercising the ssh.HostKeyCallback
// signature without a real network connection.
type fakeAddr struct{ addr string }

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return a.addr }

func TestVerifyHostKey_UnknownHostPromptsAndRemembersOnAccept(t *testing.T) {
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	key := testKey(t)

	var prompted string
	cb, err := VerifyHostKey(knownHosts, func(hostname string, k ssh.PublicKey) bool {
		prompted = hostname
		return true
	})
	if err != nil {
		t.Fatalf("VerifyHostKey: %v", err)
	}

	if err := cb("example.com:22", fakeAddr{"1.2.3.4:22"}, key); err != nil {
		t.Fatalf("callback with accepted new host: %v", err)
	}
	if prompted != "example.com:22" {
		t.Errorf("prompt hostname = %q, want %q", prompted, "example.com:22")
	}

	data, err := os.ReadFile(knownHosts)
	if err != nil {
		t.Fatalf("reading known_hosts after accept: %v", err)
	}
	if len(data) == 0 {
		t.Error("known_hosts is still empty after accepting a new host key")
	}
}

func TestVerifyHostKey_UnknownHostRefusedOnDecline(t *testing.T) {
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	key := testKey(t)

	cb, err := VerifyHostKey(knownHosts, func(hostname string, k ssh.PublicKey) bool {
		return false
	})
	if err != nil {
		t.Fatalf("VerifyHostKey: %v", err)
	}

	if err := cb("example.com:22", fakeAddr{"1.2.3.4:22"}, key); err == nil {
		t.Error("expected an error when the prompt declines the new host key")
	}

	data, err := os.ReadFile(knownHosts)
	if err != nil {
		t.Fatalf("reading known_hosts: %v", err)
	}
	if len(data) != 0 {
		t.Error("known_hosts should still be empty after declining")
	}
}

func TestVerifyHostKey_KnownMatchingKeyIsSilentlyAccepted(t *testing.T) {
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	key := testKey(t)

	promptCalls := 0
	prompt := func(hostname string, k ssh.PublicKey) bool {
		promptCalls++
		return true
	}

	cb, err := VerifyHostKey(knownHosts, prompt)
	if err != nil {
		t.Fatalf("VerifyHostKey: %v", err)
	}
	if err := cb("example.com:22", fakeAddr{"1.2.3.4:22"}, key); err != nil {
		t.Fatalf("first connect (accepting): %v", err)
	}
	if promptCalls != 1 {
		t.Fatalf("promptCalls after first connect = %d, want 1", promptCalls)
	}

	// A fresh VerifyHostKey call re-reads the same file from disk, the
	// same way a brand new process connecting later would.
	cb2, err := VerifyHostKey(knownHosts, prompt)
	if err != nil {
		t.Fatalf("VerifyHostKey (second): %v", err)
	}
	if err := cb2("example.com:22", fakeAddr{"1.2.3.4:22"}, key); err != nil {
		t.Fatalf("second connect with the same key: %v", err)
	}
	if promptCalls != 1 {
		t.Errorf("promptCalls after second connect = %d, want still 1 (no re-prompt for a known, matching key)", promptCalls)
	}
}

func TestVerifyHostKey_ChangedKeyIsRefusedWithoutPrompting(t *testing.T) {
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	originalKey := testKey(t)
	changedKey := testKey(t)

	promptCalls := 0
	prompt := func(hostname string, k ssh.PublicKey) bool {
		promptCalls++
		return true
	}

	cb, err := VerifyHostKey(knownHosts, prompt)
	if err != nil {
		t.Fatalf("VerifyHostKey: %v", err)
	}
	if err := cb("example.com:22", fakeAddr{"1.2.3.4:22"}, originalKey); err != nil {
		t.Fatalf("first connect (recording originalKey): %v", err)
	}

	cb2, err := VerifyHostKey(knownHosts, prompt)
	if err != nil {
		t.Fatalf("VerifyHostKey (second): %v", err)
	}
	err = cb2("example.com:22", fakeAddr{"1.2.3.4:22"}, changedKey)
	if err == nil {
		t.Fatal("expected an error when the host's key has changed")
	}
	if promptCalls != 1 {
		t.Errorf("promptCalls = %d, want still 1 — a changed key must never reach the accept/decline prompt", promptCalls)
	}
}

func TestVerifyHostKey_CreatesAMissingKnownHostsFileAndItsDirectory(t *testing.T) {
	dir := t.TempDir()
	knownHosts := filepath.Join(dir, "nested", "known_hosts")

	if _, err := VerifyHostKey(knownHosts, func(string, ssh.PublicKey) bool { return true }); err != nil {
		t.Fatalf("VerifyHostKey with a not-yet-existing file: %v", err)
	}
	if _, err := os.Stat(knownHosts); err != nil {
		t.Errorf("expected known_hosts to have been created: %v", err)
	}
}

var _ net.Addr = fakeAddr{}
