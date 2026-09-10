// The "Connect to server..." dialog and the glue that turns a filled-in
// form into a live Remote tab (see paneTabs.AddRemote in main.go). The
// actual SFTP/SSH work lives in internal/sftpfs; this file is UI only.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"

	Graphite "github.com/yeoblyv/graphite"

	"github.com/yeoblyv/diskette/internal/sftpfs"
	"github.com/yeoblyv/diskette/internal/tabs"
)

// authMethodLabels are the sftpfs.AuthMethod choices offered in the
// dialog's ComboBox, index-for-index with sftpfs.AuthPassword/
// AuthPrivateKey/AuthAgent.
var authMethodLabels = []string{"Password", "Private key", "SSH agent"}

// authMethodStrings is authMethodLabels' persisted-string counterpart
// (tabs.SavedRemote.AuthMethod), so a saved connection's auth method
// round-trips without internal/tabs needing to depend on internal/sftpfs
// for its own enum.
var authMethodStrings = []string{"password", "privatekey", "agent"}

func authMethodFromSaved(s string) sftpfs.AuthMethod {
	for i, name := range authMethodStrings {
		if name == s {
			return sftpfs.AuthMethod(i)
		}
	}
	return sftpfs.AuthPassword
}

// guessKeyPath returns the first conventional private key file that
// exists under ~/.ssh, or "" if none do — a convenience default for the
// dialog's key-path field, not a requirement (the user can always type a
// different path).
func guessKeyPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, name := range []string{"id_ed25519", "id_rsa", "id_ecdsa"} {
		p := filepath.Join(home, ".ssh", name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// showConnectDialog opens the "Connect to server" modal, which on a
// successful connection adds a Remote tab to target via AddRemote. If
// prefill is non-nil, every field except the secret (password/passphrase,
// which is never saved) is pre-populated from it — used to reconnect a
// pinned Remote tab on restart (see restoreSide in main.go) — and
// onConnected, if non-nil, runs right after AddRemote succeeds so the
// caller can mark the new tab pinned/renamed to match what was restored
// (AddRemote itself has no way to know this is a restore rather than a
// fresh connection).
func showConnectDialog(app *Graphite.Application, target *paneTabs, prefill *tabs.SavedRemote, onConnected func()) {
	mod := Graphite.NewWindow(60, 28, " Connect to server ")

	hostBox := Graphite.NewInputBox(2, 1, 54, "Host:      ")
	portBox := Graphite.NewInputBox(2, 3, 54, "Port:      ")
	portBox.Value = "22"
	portBox.CursorPos = len(portBox.Value)
	userBox := Graphite.NewInputBox(2, 5, 54, "Username:  ")

	authRow := Graphite.NewFlex(2, 7, 54, 1, Graphite.FlexRow)
	authRow.Gap = 1
	authCombo := Graphite.NewComboBox(0, 0, 20, authMethodLabels, nil)

	keyPathBox := Graphite.NewInputBox(0, 0, 54, "Key file:  ")
	keyPathBox.Value = guessKeyPath()
	keyPathBox.CursorPos = len([]rune(keyPathBox.Value))
	passphraseBox := Graphite.NewPasswordBox(0, 0, 54, "Passphrase:")
	passwordBox := Graphite.NewPasswordBox(0, 0, 54, "Password:  ")

	if prefill != nil {
		hostBox.Value = prefill.Host
		hostBox.CursorPos = len([]rune(hostBox.Value))
		if prefill.Port != 0 {
			portBox.Value = strconv.Itoa(prefill.Port)
			portBox.CursorPos = len(portBox.Value)
		}
		userBox.Value = prefill.Username
		userBox.CursorPos = len([]rune(userBox.Value))
		if prefill.KeyPath != "" {
			keyPathBox.Value = prefill.KeyPath
			keyPathBox.CursorPos = len([]rune(keyPathBox.Value))
		}
		authCombo.Selected = int(authMethodFromSaved(prefill.AuthMethod))
	}

	// secretFlex holds only the rows whose visibility depends on the
	// chosen auth method, so toggling Visible on one of them reflows the
	// others to close the gap (Flex's own "invisible children get
	// neither drawn nor a share of space" behavior — see graphite's
	// flex.go) instead of leaving a blank row where a hidden field was.
	secretFlex := Graphite.NewFlex(2, 9, 54, -4, Graphite.FlexColumn)
	secretFlex.Gap = 1
	secretFlex.AddChild(keyPathBox, 0)
	secretFlex.AddChild(passphraseBox, 0)
	secretFlex.AddChild(passwordBox, 0)

	applyAuthVisibility := func() {
		switch sftpfs.AuthMethod(authCombo.Selected) {
		case sftpfs.AuthPrivateKey:
			keyPathBox.Visible, passphraseBox.Visible, passwordBox.Visible = true, true, false
		case sftpfs.AuthAgent:
			keyPathBox.Visible, passphraseBox.Visible, passwordBox.Visible = false, false, false
		default: // AuthPassword
			keyPathBox.Visible, passphraseBox.Visible, passwordBox.Visible = false, false, true
		}
	}
	authCombo.OnSelect = func(int, string) { applyAuthVisibility() }
	applyAuthVisibility()
	authRow.AddChild(Graphite.NewLabel(0, 0, "Auth method:"), 0)
	authRow.AddChild(authCombo, 0)

	mod.AddWidget(hostBox)
	mod.AddWidget(portBox)
	mod.AddWidget(userBox)
	mod.AddWidget(authRow)
	mod.AddWidget(secretFlex)

	connecting := false
	doConnect := func() {
		if connecting {
			return
		}
		portText := strings.TrimSpace(portBox.Value)
		port := 22
		if portText != "" {
			p, err := strconv.Atoi(portText)
			if err != nil {
				app.ShowMessage(" Error ", "Port must be a number.", Graphite.BtnDanger)
				return
			}
			port = p
		}
		host := strings.TrimSpace(hostBox.Value)
		if host == "" {
			app.ShowMessage(" Error ", "Host cannot be empty.", Graphite.BtnDanger)
			return
		}

		cfg := sftpfs.Config{
			Host:          host,
			Port:          port,
			Username:      strings.TrimSpace(userBox.Value),
			AuthMethod:    sftpfs.AuthMethod(authCombo.Selected),
			Password:      passwordBox.Value,
			KeyPath:       strings.TrimSpace(keyPathBox.Value),
			KeyPassphrase: passphraseBox.Value,
		}
		meta := tabs.SavedRemote{
			Host: cfg.Host, Port: cfg.Port, Username: cfg.Username,
			AuthMethod: authMethodStrings[cfg.AuthMethod], KeyPath: cfg.KeyPath,
		}

		connecting = true
		connectRemote(app, target, cfg, meta, func() { connecting = false }, onConnected)
	}

	hostBox.OnSubmit = func(string) { doConnect() }
	portBox.OnSubmit = func(string) { doConnect() }
	userBox.OnSubmit = func(string) { doConnect() }
	passwordBox.OnSubmit = func(string) { doConnect() }
	passphraseBox.OnSubmit = func(string) { doConnect() }

	mod.AddWidget(Graphite.NewButton(2, -2, "Connect", Graphite.BtnSuccess, doConnect))
	mod.AddWidget(Graphite.NewButton(14, -2, "Cancel", Graphite.BtnDefault, func() {
		// Refuses to close while a dial is in flight: connectRemote's own
		// completion handler closes this exact modal by calling
		// app.CloseModal() unconditionally once it resolves, which would
		// otherwise pop whatever modal happens to be on top by then —
		// not necessarily this one anymore — if this button let the user
		// dismiss it first.
		if !connecting {
			app.CloseModal()
		}
	}))

	app.SetModal(mod)
}

// connectRemote does the actual dialing on a background goroutine —
// reading/appending ~/.ssh/known_hosts, the SSH handshake, and opening
// the SFTP session are all real I/O — and hops back to the main loop for
// every UI-touching step (a host-key prompt mid-dial, the final success
// or failure). done runs when the attempt finishes either way (clearing
// showConnectDialog's re-entrancy guard); onConnected, if non-nil, runs
// only after a successful AddRemote.
func connectRemote(app *Graphite.Application, target *paneTabs, cfg sftpfs.Config, meta tabs.SavedRemote, done func(), onConnected func()) {
	go func() {
		defer app.Invoke(done)

		knownHostsPath, err := sftpfs.DefaultKnownHostsPath()
		if err != nil {
			app.Invoke(func() { app.ShowMessage(" Error ", err.Error(), Graphite.BtnDanger) })
			return
		}

		prompt := func(hostname string, key ssh.PublicKey) bool {
			answer := make(chan bool, 1)
			app.Invoke(func() {
				showHostKeyConfirm(app, hostname, key, func(accepted bool) { answer <- accepted })
			})
			return <-answer
		}

		hostKeyCB, err := sftpfs.VerifyHostKey(knownHostsPath, prompt)
		if err != nil {
			app.Invoke(func() { app.ShowMessage(" Error ", err.Error(), Graphite.BtnDanger) })
			return
		}

		fs, err := sftpfs.Dial(context.Background(), cfg, hostKeyCB)
		app.Invoke(func() {
			if err != nil {
				app.ShowMessage(" Error ", err.Error(), Graphite.BtnDanger)
				return
			}
			app.CloseModal() // the connect dialog
			label := cfg.Username + "@" + cfg.Host
			target.AddRemote(fs, "/", label, meta)
			if onConnected != nil {
				onConnected()
			}
		})
	}()
}

// showHostKeyConfirm asks whether to trust hostname's offered key,
// showing its type and SHA256 fingerprint — the same information a real
// ssh client's own first-connection prompt shows. respond is called
// exactly once, from the main loop (this is itself shown via app.Invoke
// by the caller) — a hand-built modal rather than Graphite.ShowConfirm,
// since ShowConfirm's "No" button only ever calls CloseModal with no way
// to also run a decline callback, and connectRemote's prompt goroutine is
// blocked on respond being called either way.
func showHostKeyConfirm(app *Graphite.Application, hostname string, key ssh.PublicKey, respond func(bool)) {
	msg := fmt.Sprintf(
		"The authenticity of host '%s' can't be established.\n%s key fingerprint:\n%s\n\nTrust this key and continue connecting?",
		hostname, key.Type(), ssh.FingerprintSHA256(key),
	)
	lines := len(strings.Split(msg, "\n"))
	winH := lines + 6
	if winH < 10 {
		winH = 10
	}

	mod := Graphite.NewWindow(56, winH, " Unknown Host ")
	mod.AddWidget(Graphite.NewLabel(2, 1, msg))
	mod.AddWidget(Graphite.NewButton(2, -2, "Trust", Graphite.BtnDanger, func() {
		app.CloseModal()
		respond(true)
	}))
	mod.AddWidget(Graphite.NewButton(14, -2, "Cancel", Graphite.BtnDefault, func() {
		app.CloseModal()
		respond(false)
	}))
	app.SetModal(mod)
}
