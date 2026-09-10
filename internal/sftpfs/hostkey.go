package sftpfs

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// HostKeyPrompt is called when a server's host key isn't in the known_hosts
// file at all yet — the normal case the very first time diskette connects
// to a given host. It's shown the hostname (as it will be recorded) and
// the offered key, and returns whether to trust and remember it. It must
// not block on anything other than obtaining that answer itself — same
// contract as copyengine.ResolveFunc, and for the same reason: this runs
// on whatever goroutine is dialing, not the UI's own main loop.
type HostKeyPrompt func(hostname string, key ssh.PublicKey) bool

// DefaultKnownHostsPath returns the real ~/.ssh/known_hosts — the same
// file the user's own ssh/scp/sftp command-line tools already trust, so
// connecting through diskette doesn't create a second, parallel trust
// store to keep in sync.
func DefaultKnownHostsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh", "known_hosts"), nil
}

// VerifyHostKey builds an ssh.HostKeyCallback backed by the known_hosts
// file at path (see DefaultKnownHostsPath):
//
//   - a key that matches what's already recorded for this host is
//     accepted silently, same as a real ssh client;
//   - a host with no entry at all calls prompt; accepting appends a new
//     line to the real known_hosts file (creating it, and its parent
//     directory, if this is the very first host diskette or the user's
//     own ssh has ever connected to), refusing fails the connection;
//   - a host whose recorded key doesn't match the one just offered is
//     refused outright, no prompt offered — this is the "the remote host
//     identification has changed" case a real ssh client treats as a
//     possible MITM attack, and there is deliberately no "trust it
//     anyway" shortcut in the UI for this phase.
func VerifyHostKey(path string, prompt HostKeyPrompt) (ssh.HostKeyCallback, error) {
	if err := ensureFileExists(path); err != nil {
		return nil, err
	}
	base, err := knownhosts.New(path)
	if err != nil {
		return nil, err
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		return checkHostKey(base, path, hostname, remote, key, prompt)
	}, nil
}

// checkHostKey is VerifyHostKey's callback body, split out so the
// unknown/changed/matching branching reads clearly on its own.
func checkHostKey(base ssh.HostKeyCallback, knownHostsPath, hostname string, remote net.Addr, key ssh.PublicKey, prompt HostKeyPrompt) error {
	err := base(hostname, remote, key)
	if err == nil {
		return nil // known and matching
	}

	keyErr, ok := err.(*knownhosts.KeyError)
	if !ok {
		return err
	}
	if len(keyErr.Want) > 0 {
		// Known host, key changed: never offer a click-through here.
		return fmt.Errorf("host key for %s has changed since it was last recorded — refusing to connect (this can mean the server was reinstalled, or a man-in-the-middle attack): %w", hostname, err)
	}

	// Unknown host.
	if prompt == nil || !prompt(hostname, key) {
		return fmt.Errorf("host key for %s was not accepted", hostname)
	}
	return appendKnownHost(knownHostsPath, hostname, key)
}

// appendKnownHost records key for hostname in the known_hosts file at
// path, in the exact line format knownhosts.New itself parses back.
func appendKnownHost(path, hostname string, key ssh.PublicKey) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
	_, err = fmt.Fprintln(f, line)
	return err
}

// ensureFileExists creates path (and its parent directory) as an empty
// file if it doesn't already exist, so a user who has never made an SSH
// connection before — no ~/.ssh directory, no known_hosts at all — gets
// treated as "every host is unknown" (prompted normally) rather than
// knownhosts.New failing outright on a missing file.
func ensureFileExists(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}
