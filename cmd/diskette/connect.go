// The "Connect to server..." dialog and the glue that turns a filled-in
// form into a live Remote tab (see paneTabs.AddRemote in main.go). The
// actual SFTP/SSH and FTP/FTPS work lives in internal/sftpfs and
// internal/ftpfs respectively; this file is UI only.
package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"

	Graphite "github.com/yeoblyv/graphite"

	"github.com/yeoblyv/diskette/internal/ftpfs"
	"github.com/yeoblyv/diskette/internal/locales"
	"github.com/yeoblyv/diskette/internal/sftpfs"
	"github.com/yeoblyv/diskette/internal/tabs"
)

// protocolLabels are the dialog's top-level Protocol choices, index-for-
// index with the sftp/ftp sections toggled below — protocol names, never
// translated (like "SFTP"/"FTP" anywhere else in the UI).
var protocolLabels = []string{"SFTP", "FTP"}

// authMethodLabels returns the sftpfs.AuthMethod choices offered in the
// dialog's ComboBox, index-for-index with sftpfs.AuthPassword/
// AuthPrivateKey/AuthAgent, translated for app's current locale — a
// function rather than a package-level var (unlike protocolLabels, which
// never changes) since these three actually are ordinary words.
func authMethodLabels(app *Graphite.Application) []string {
	return []string{app.T(locales.KeyAuthPassword), app.T(locales.KeyAuthPrivateKey), app.T(locales.KeyAuthAgent)}
}

// authMethodStrings is authMethodLabels' persisted-string counterpart
// (tabs.SavedRemote.AuthMethod), so a saved connection's auth method
// round-trips without internal/tabs needing to depend on internal/sftpfs
// for its own enum.
var authMethodStrings = []string{"password", "privatekey", "agent"}

// ftpTLSLabels returns the dialog's FTP Security choices, index-for-index
// with ftpfs.TLSNone/TLSExplicit/TLSImplicit, translated for app's
// current locale — see authMethodLabels.
func ftpTLSLabels(app *Graphite.Application) []string {
	return []string{app.T(locales.KeyTLSNone), app.T(locales.KeyTLSExplicit), app.T(locales.KeyTLSImplicit)}
}

// ftpTLSStrings is ftpTLSLabels' persisted-string counterpart
// (tabs.SavedRemote.TLSMode), for the same reason authMethodStrings
// exists.
var ftpTLSStrings = []string{"none", "explicit", "implicit"}

func authMethodFromSaved(s string) sftpfs.AuthMethod {
	for i, name := range authMethodStrings {
		if name == s {
			return sftpfs.AuthMethod(i)
		}
	}
	return sftpfs.AuthPassword
}

func ftpTLSModeFromSaved(s string) ftpfs.TLSMode {
	for i, name := range ftpTLSStrings {
		if name == s {
			return ftpfs.TLSMode(i)
		}
	}
	return ftpfs.TLSNone
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
// successful connection adds a Remote tab to target via AddRemote, opened
// at startPath (use "/" for a fresh connection; doReconnect passes the
// path the old session was showing, so reconnecting doesn't lose your
// place). If prefill is non-nil, every field except the secret
// (password/passphrase, which is never saved) is pre-populated from it,
// including which protocol section is shown — used both by
// restoreSide (restarting into a pinned Remote tab) and doReconnect — and
// onConnected, if non-nil, runs right after AddRemote succeeds so the
// caller can mark the new tab pinned/renamed/positioned to match
// whatever it's replacing (AddRemote itself has no way to know that).
func showConnectDialog(app *Graphite.Application, target *paneTabs, prefill *tabs.SavedRemote, startPath string, onConnected func()) {
	mod := Graphite.NewWindow(60, 30, app.T(locales.KeyConnectTitle))

	protocolRow := Graphite.NewFlex(2, 1, 54, 1, Graphite.FlexRow)
	protocolRow.Gap = 1
	protocolCombo := Graphite.NewComboBox(0, 0, 16, protocolLabels, nil)
	protocolRow.AddChild(Graphite.NewLabel(0, 0, app.T(locales.KeyConnectProtocol)), 0)
	protocolRow.AddChild(protocolCombo, 0)
	mod.AddWidget(protocolRow)

	// --- SFTP section ---

	sftpHostBox := Graphite.NewInputBox(0, 0, 54, app.T(locales.KeyConnectHost))
	sftpPortBox := Graphite.NewInputBox(0, 0, 54, app.T(locales.KeyConnectPort))
	sftpPortBox.Value = "22"
	sftpPortBox.CursorPos = len(sftpPortBox.Value)
	sftpUserBox := Graphite.NewInputBox(0, 0, 54, app.T(locales.KeyConnectUsername))

	authRow := Graphite.NewFlex(0, 0, 54, 1, Graphite.FlexRow)
	authRow.Gap = 1
	authCombo := Graphite.NewComboBox(0, 0, 20, authMethodLabels(app), nil)

	keyPathBox := Graphite.NewInputBox(0, 0, 54, app.T(locales.KeyConnectKeyFile))
	keyPathBox.Value = guessKeyPath()
	keyPathBox.CursorPos = len([]rune(keyPathBox.Value))
	passphraseBox := Graphite.NewPasswordBox(0, 0, 54, app.T(locales.KeyConnectPassphrase))
	passwordBox := Graphite.NewPasswordBox(0, 0, 54, app.T(locales.KeyConnectPassword))

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
	authRow.AddChild(Graphite.NewLabel(0, 0, app.T(locales.KeyConnectAuthMethod)), 0)
	authRow.AddChild(authCombo, 0)

	// secretFlex holds only the rows whose visibility depends on the
	// chosen auth method, so toggling Visible on one of them reflows the
	// others to close the gap (Flex's own "invisible children get
	// neither drawn nor a share of space" behavior — see graphite's
	// flex.go) instead of leaving a blank row where a hidden field was.
	secretFlex := Graphite.NewFlex(0, 0, 54, 1, Graphite.FlexColumn)
	secretFlex.Gap = 1
	secretFlex.AddChild(keyPathBox, 0)
	secretFlex.AddChild(passphraseBox, 0)
	secretFlex.AddChild(passwordBox, 0)

	// sftpSection stacks every SFTP field as one Flex, so hiding the
	// whole section (switching Protocol to FTP) is a single Visible
	// toggle rather than one per field. secretFlex is the only
	// weight>0 child, absorbing whatever height the four fixed rows
	// above it don't use.
	sftpSection := Graphite.NewFlex(2, 3, 54, -4, Graphite.FlexColumn)
	sftpSection.Gap = 1
	sftpSection.AddChild(sftpHostBox, 0)
	sftpSection.AddChild(sftpPortBox, 0)
	sftpSection.AddChild(sftpUserBox, 0)
	sftpSection.AddChild(authRow, 0)
	sftpSection.AddChild(secretFlex, 1)
	mod.AddWidget(sftpSection)

	// --- FTP section ---

	ftpHostBox := Graphite.NewInputBox(0, 0, 54, app.T(locales.KeyConnectHost))
	ftpPortBox := Graphite.NewInputBox(0, 0, 54, app.T(locales.KeyConnectPort))
	ftpPortBox.Value = "21"
	ftpPortBox.CursorPos = len(ftpPortBox.Value)
	ftpUserBox := Graphite.NewInputBox(0, 0, 54, app.T(locales.KeyConnectUsername))
	ftpPasswordBox := Graphite.NewPasswordBox(0, 0, 54, app.T(locales.KeyConnectPassword))

	securityRow := Graphite.NewFlex(0, 0, 54, 1, Graphite.FlexRow)
	securityRow.Gap = 1
	securityCombo := Graphite.NewComboBox(0, 0, 20, ftpTLSLabels(app), nil)
	securityRow.AddChild(Graphite.NewLabel(0, 0, app.T(locales.KeyConnectSecurity)), 0)
	securityRow.AddChild(securityCombo, 0)

	ftpSection := Graphite.NewFlex(2, 3, 54, -4, Graphite.FlexColumn)
	ftpSection.Gap = 1
	ftpSection.AddChild(ftpHostBox, 0)
	ftpSection.AddChild(ftpPortBox, 0)
	ftpSection.AddChild(ftpUserBox, 0)
	ftpSection.AddChild(securityRow, 0)
	ftpSection.AddChild(ftpPasswordBox, 0)
	mod.AddWidget(ftpSection)

	applyProtocolVisibility := func() {
		isFTP := protocolCombo.Selected == 1
		sftpSection.Visible = !isFTP
		ftpSection.Visible = isFTP
	}
	protocolCombo.OnSelect = func(int, string) { applyProtocolVisibility() }

	// --- prefill ---

	protocolCombo.Selected = 0
	if prefill != nil && prefill.Protocol == "ftp" {
		protocolCombo.Selected = 1
	}
	applyProtocolVisibility()

	if prefill != nil {
		switch prefill.Protocol {
		case "ftp":
			ftpHostBox.Value = prefill.Host
			ftpHostBox.CursorPos = len([]rune(ftpHostBox.Value))
			if prefill.Port != 0 {
				ftpPortBox.Value = strconv.Itoa(prefill.Port)
				ftpPortBox.CursorPos = len(ftpPortBox.Value)
			}
			ftpUserBox.Value = prefill.Username
			ftpUserBox.CursorPos = len([]rune(ftpUserBox.Value))
			securityCombo.Selected = int(ftpTLSModeFromSaved(prefill.TLSMode))
		default: // "sftp", or empty (a tab saved before Protocol existed)
			sftpHostBox.Value = prefill.Host
			sftpHostBox.CursorPos = len([]rune(sftpHostBox.Value))
			if prefill.Port != 0 {
				sftpPortBox.Value = strconv.Itoa(prefill.Port)
				sftpPortBox.CursorPos = len(sftpPortBox.Value)
			}
			sftpUserBox.Value = prefill.Username
			sftpUserBox.CursorPos = len([]rune(sftpUserBox.Value))
			if prefill.KeyPath != "" {
				keyPathBox.Value = prefill.KeyPath
				keyPathBox.CursorPos = len([]rune(keyPathBox.Value))
			}
			authCombo.Selected = int(authMethodFromSaved(prefill.AuthMethod))
			applyAuthVisibility()
		}
	}

	// --- connect ---

	connecting := false
	doConnect := func() {
		if connecting {
			return
		}
		if protocolCombo.Selected == 1 {
			connectFTPFromDialog(app, target, startPath, ftpHostBox, ftpPortBox, ftpUserBox, ftpPasswordBox, securityCombo, &connecting, onConnected)
		} else {
			connectSFTPFromDialog(app, target, startPath, sftpHostBox, sftpPortBox, sftpUserBox, authCombo, keyPathBox, passphraseBox, passwordBox, &connecting, onConnected)
		}
	}

	sftpHostBox.OnSubmit = func(string) { doConnect() }
	sftpPortBox.OnSubmit = func(string) { doConnect() }
	sftpUserBox.OnSubmit = func(string) { doConnect() }
	passwordBox.OnSubmit = func(string) { doConnect() }
	passphraseBox.OnSubmit = func(string) { doConnect() }
	ftpHostBox.OnSubmit = func(string) { doConnect() }
	ftpPortBox.OnSubmit = func(string) { doConnect() }
	ftpUserBox.OnSubmit = func(string) { doConnect() }
	ftpPasswordBox.OnSubmit = func(string) { doConnect() }

	connectLabel := app.T(locales.KeyConnectButton)
	mod.AddWidget(Graphite.NewButton(2, -2, connectLabel, Graphite.BtnSuccess, doConnect))
	mod.AddWidget(Graphite.NewButton(nextButtonX(2, connectLabel), -2, app.T(locales.KeyCancel), Graphite.BtnDefault, func() {
		// Refuses to close while a dial is in flight: the connect
		// goroutine's own completion handler closes this exact modal by
		// calling app.CloseModal() unconditionally once it resolves,
		// which would otherwise pop whatever modal happens to be on top
		// by then — not necessarily this one anymore — if this button
		// let the user dismiss it first.
		if !connecting {
			app.CloseModal()
		}
	}))

	app.SetModal(mod)
}

// parsePort trims and parses an *InputBox's port field, defaulting to
// def when the field is blank; ok is false (and an error already shown)
// when it's non-blank but not a number.
func parsePort(app *Graphite.Application, box *Graphite.InputBox, def int) (port int, ok bool) {
	text := strings.TrimSpace(box.Value)
	if text == "" {
		return def, true
	}
	p, err := strconv.Atoi(text)
	if err != nil {
		app.ShowMessage(app.T(locales.KeyErrorTitle), app.T(locales.KeyErrPortNumber), Graphite.BtnDanger)
		return 0, false
	}
	return p, true
}

// connectSFTPFromDialog reads the SFTP section's fields, validates them,
// and starts the SFTP connect+host-key flow — the dialog-facing half of
// what connectRemote/verifyAndDial used to do inline.
func connectSFTPFromDialog(app *Graphite.Application, target *paneTabs, startPath string, hostBox, portBox, userBox *Graphite.InputBox, authCombo *Graphite.ComboBox, keyPathBox, passphraseBox, passwordBox *Graphite.InputBox, connecting *bool, onConnected func()) {
	port, ok := parsePort(app, portBox, 22)
	if !ok {
		return
	}
	host := strings.TrimSpace(hostBox.Value)
	if host == "" {
		app.ShowMessage(app.T(locales.KeyErrorTitle), app.T(locales.KeyErrHostEmpty), Graphite.BtnDanger)
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
		Protocol: "sftp", Host: cfg.Host, Port: cfg.Port, Username: cfg.Username,
		AuthMethod: authMethodStrings[cfg.AuthMethod], KeyPath: cfg.KeyPath,
	}

	*connecting = true
	connectSFTP(app, target, startPath, cfg, meta, func() { *connecting = false }, onConnected)
}

// connectFTPFromDialog is connectSFTPFromDialog's FTP counterpart.
func connectFTPFromDialog(app *Graphite.Application, target *paneTabs, startPath string, hostBox, portBox, userBox, passwordBox *Graphite.InputBox, securityCombo *Graphite.ComboBox, connecting *bool, onConnected func()) {
	port, ok := parsePort(app, portBox, 21)
	if !ok {
		return
	}
	host := strings.TrimSpace(hostBox.Value)
	if host == "" {
		app.ShowMessage(app.T(locales.KeyErrorTitle), app.T(locales.KeyErrHostEmpty), Graphite.BtnDanger)
		return
	}

	tlsMode := ftpfs.TLSMode(securityCombo.Selected)
	cfg := ftpfs.Config{
		Host: host, Port: port,
		Username: strings.TrimSpace(userBox.Value),
		Password: passwordBox.Value,
		TLSMode:  tlsMode,
	}
	meta := tabs.SavedRemote{
		Protocol: "ftp", Host: cfg.Host, Port: cfg.Port, Username: cfg.Username,
		TLSMode: ftpTLSStrings[tlsMode],
	}

	*connecting = true
	connectFTP(app, target, startPath, cfg, meta, func() { *connecting = false }, onConnected)
}

// connectSFTP does the actual SFTP dialing on a background goroutine —
// reading/appending ~/.ssh/known_hosts, the SSH handshake, and opening
// the SFTP session are all real I/O — and hops back to the main loop for
// every UI-touching step (a host-key prompt mid-dial, the final success
// or failure). done runs when the attempt finishes either way (clearing
// showConnectDialog's re-entrancy guard); onConnected, if non-nil, runs
// only after a successful AddRemote.
func connectSFTP(app *Graphite.Application, target *paneTabs, startPath string, cfg sftpfs.Config, meta tabs.SavedRemote, done func(), onConnected func()) {
	go func() {
		defer app.Invoke(done)

		knownHostsPath, err := sftpfs.DefaultKnownHostsPath()
		if err != nil {
			app.Invoke(func() { app.ShowMessage(app.T(locales.KeyErrorTitle), err.Error(), Graphite.BtnDanger) })
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
			app.Invoke(func() { app.ShowMessage(app.T(locales.KeyErrorTitle), err.Error(), Graphite.BtnDanger) })
			return
		}

		fs, err := sftpfs.Dial(context.Background(), cfg, hostKeyCB)
		app.Invoke(func() {
			if err != nil {
				app.ShowMessage(app.T(locales.KeyErrorTitle), err.Error(), Graphite.BtnDanger)
				return
			}
			app.CloseModal() // the connect dialog
			label := cfg.Username + "@" + cfg.Host
			target.AddRemote(fs, startPath, label, meta)
			if onConnected != nil {
				onConnected()
			}
		})
	}()
}

// connectFTP is connectSFTP's FTP/FTPS counterpart — simpler, since FTP
// has no host-key concept: TLS certificate verification (for FTPS) uses
// the standard system trust store with no equivalent trust-on-first-use
// prompt, so an untrusted/self-signed certificate just surfaces as a
// plain connect error.
func connectFTP(app *Graphite.Application, target *paneTabs, startPath string, cfg ftpfs.Config, meta tabs.SavedRemote, done func(), onConnected func()) {
	go func() {
		defer app.Invoke(done)

		fs, err := ftpfs.Dial(context.Background(), cfg)
		app.Invoke(func() {
			if err != nil {
				app.ShowMessage(app.T(locales.KeyErrorTitle), err.Error(), Graphite.BtnDanger)
				return
			}
			app.CloseModal() // the connect dialog
			label := cfg.Username + "@" + cfg.Host
			target.AddRemote(fs, startPath, label, meta)
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
// to also run a decline callback, and connectSFTP's prompt goroutine is
// blocked on respond being called either way.
func showHostKeyConfirm(app *Graphite.Application, hostname string, key ssh.PublicKey, respond func(bool)) {
	msg := app.T(locales.KeyHostKeyMessage, hostname, key.Type(), ssh.FingerprintSHA256(key))
	lines := len(strings.Split(msg, "\n"))
	winH := lines + 6
	if winH < 10 {
		winH = 10
	}

	mod := Graphite.NewWindow(56, winH, app.T(locales.KeyHostKeyTitle))
	mod.AddWidget(Graphite.NewLabel(2, 1, msg))
	trustLabel := app.T(locales.KeyHostKeyTrust)
	mod.AddWidget(Graphite.NewButton(2, -2, trustLabel, Graphite.BtnDanger, func() {
		app.CloseModal()
		respond(true)
	}))
	mod.AddWidget(Graphite.NewButton(nextButtonX(2, trustLabel), -2, app.T(locales.KeyCancel), Graphite.BtnDefault, func() {
		app.CloseModal()
		respond(false)
	}))
	app.SetModal(mod)
}

// doReconnect implements the Network menu's "Reconnect": re-opens the
// Connect dialog pre-filled from the active tab's own connection
// metadata, at the same path it was showing, without touching the old
// connection until a new one is actually established. Canceling the
// dialog leaves the original tab exactly as it was — nothing is closed
// or removed up front, only once AddRemote has already added its
// replacement, so there's no window where the pane is left with a dead
// connection or (on a pane's only tab) no tab at all.
func doReconnect(app *Graphite.Application, p *paneTabs) {
	idx := p.group.Active
	content, ok := p.contentAt(idx)
	if !ok || content.remote == nil {
		app.ShowMessage(app.T(locales.KeyErrorTitle), app.T(locales.KeyErrNotConnected), Graphite.BtnDanger)
		return
	}

	meta := content.remote.meta
	oldFS := content.remote.fs
	pinned := p.group.Tabs[idx].Pinned
	name := p.group.Tabs[idx].Name
	startPath := content.filePane.Path()

	showConnectDialog(app, p, &meta, startPath, func() {
		newIdx := p.group.Active
		p.group.Tabs[newIdx].Pinned = pinned
		p.group.Tabs[newIdx].Name = name
		oldFS.Close()
		// idx is still valid here: AddRemote only ever appends, so the
		// old tab hasn't moved, and there are now at least two tabs
		// (the old one plus the new one just added), so removing it is
		// never refused as "a pane's last tab."
		p.group.Close(idx)
	})
}

// doDisconnect implements the Network menu's "Disconnect": closes the
// active tab's connection. If other tabs remain on the pane, this is
// exactly CloseTabAt on the active tab; if it's the pane's only tab (which
// CloseTabAt refuses to close, since a pane must always show something),
// the connection is closed and the tab replaced with a local listing at
// the process's own working directory instead of leaving the pane with
// nothing open.
func doDisconnect(app *Graphite.Application, p *paneTabs) {
	idx := p.group.Active
	content, ok := p.contentAt(idx)
	if !ok || content.remote == nil {
		app.ShowMessage(app.T(locales.KeyErrorTitle), app.T(locales.KeyErrNotConnected), Graphite.BtnDanger)
		return
	}

	if len(p.group.Tabs) > 1 {
		p.CloseTabAt(idx)
		return
	}
	content.remote.fs.Close()
	p.AddFileList(mustGetwd())
	p.group.Close(idx) // safe now: AddFileList just added a second tab
}
