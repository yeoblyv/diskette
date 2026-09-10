// IPC lets a shell running inside one of diskette's own Terminal tabs
// talk back to the diskette process hosting it — the `diskette view`/
// `diskette sync`/`diskette tag` commands documented in cli.go. The
// terminal's shell is a separate OS process, so this needs a real
// transport: a loopback TCP listener diskette opens at startup, with no
// platform-specific code (unlike a Unix socket vs. a Windows named pipe),
// consistent with keeping this dependency-free like the Terminal widget
// itself. Each Terminal tab gets a random ID and the listener's address
// via environment variables at spawn time (see newTerminalContent); a
// `diskette <cmd>` invocation inside that shell reads them back out to
// address its request, then exits without ever starting the TUI.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	Graphite "github.com/yeoblyv/graphite"
)

// ipcRequest is one line of newline-delimited JSON sent over the IPC
// connection.
type ipcRequest struct {
	TerminalID string   `json:"terminalId"`
	Cmd        string   `json:"cmd"`
	Args       []string `json:"args"`
}

// ipcResponse is the single reply written back before the connection
// closes.
type ipcResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// terminalRegistry maps a Terminal tab's random ID to the paneTabs that
// owns it, so a request naming that ID can find both "which pane sent
// this" and (via otherGroup) "which pane it should act on." Reads happen
// on the IPC goroutine, writes on the main goroutine (AddTerminal/
// CloseTabAt are only ever called from UI-driven code) — the mutex is
// what makes that safe.
type terminalRegistry struct {
	mu   sync.Mutex
	byID map[string]*paneTabs
}

func newTerminalRegistry() *terminalRegistry {
	return &terminalRegistry{byID: make(map[string]*paneTabs)}
}

func (r *terminalRegistry) register(id string, p *paneTabs) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[id] = p
}

func (r *terminalRegistry) unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, id)
}

func (r *terminalRegistry) lookup(id string) (*paneTabs, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byID[id]
	return p, ok
}

// newTerminalID returns a short random hex string, unique enough for the
// lifetime of one diskette process (the only scope a registry lookup
// ever needs) without a shared counter.
func newTerminalID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing at all is exceedingly unlikely on any real
		// system; a timestamp-derived fallback still keeps IDs distinct
		// enough for this process's own short-lived registry.
		return fmt.Sprintf("t%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// startIPCServer opens the loopback listener and starts accepting
// connections in the background, returning its address (for
// DISKETTE_IPC) or an error if the platform's loopback interface somehow
// isn't available.
func startIPCServer(app *Graphite.Application, reg *terminalRegistry, otherGroup func(*paneTabs) *paneTabs) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // the listener was closed (process exiting)
			}
			go handleIPCConn(app, reg, otherGroup, conn)
		}
	}()
	return ln.Addr().String(), nil
}

// handleIPCConn services exactly one request: decode, run it on the main
// loop (via Application.Invoke, since it touches widget state), encode
// the result, close. A 5s deadline keeps a slow or hung client (or one
// that never writes/reads at all) from leaking a goroutine forever.
func handleIPCConn(app *Graphite.Application, reg *terminalRegistry, otherGroup func(*paneTabs) *paneTabs, conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	var req ipcRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		json.NewEncoder(conn).Encode(ipcResponse{Error: err.Error()})
		return
	}

	done := make(chan ipcResponse, 1)
	app.Invoke(func() {
		done <- dispatchIPC(reg, otherGroup, req)
	})
	json.NewEncoder(conn).Encode(<-done)
}

// dispatchIPC runs req against whichever pane's paneTabs isn't the one
// req.TerminalID belongs to — "the other pane" in every command's own
// documentation (cli.go) — always on the main goroutine.
func dispatchIPC(reg *terminalRegistry, otherGroup func(*paneTabs) *paneTabs, req ipcRequest) ipcResponse {
	owner, ok := reg.lookup(req.TerminalID)
	if !ok {
		return ipcResponse{Error: "this terminal tab is no longer known to diskette (was it closed?)"}
	}
	target := otherGroup(owner)

	switch req.Cmd {
	case "view":
		return ipcView(target, req.Args)
	case "tag", "untag":
		return ipcTagging(target, req.Cmd == "tag", req.Args)
	case "select":
		return ipcSelect(target, req.Args)
	default:
		return ipcResponse{Error: fmt.Sprintf("unknown command %q", req.Cmd)}
	}
}

func ipcView(target *paneTabs, args []string) ipcResponse {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}
	if _, err := os.Stat(path); err != nil {
		return ipcResponse{Error: err.Error()}
	}
	if fp, ok := target.ActiveFilePane(); ok {
		fp.SetPath(path)
	} else {
		// The other pane's active tab is a Terminal, not a FileList —
		// open a fresh FileList tab there rather than failing outright.
		target.AddFileList(path)
	}
	return ipcResponse{OK: true}
}

// ipcTagging tags or untags every named entry (by base name, matched
// against the target pane's current listing) that's actually found;
// unmatched names are reported but don't fail the ones that did match.
func ipcTagging(target *paneTabs, tag bool, names []string) ipcResponse {
	fp, ok := target.ActiveFilePane()
	if !ok {
		return ipcResponse{Error: "the other pane isn't showing a file list"}
	}
	if len(names) == 0 {
		return ipcResponse{Error: "no names given"}
	}
	var missing []string
	for _, name := range names {
		if !fp.SetTagByName(name, tag) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return ipcResponse{Error: fmt.Sprintf("not found in the other pane's current listing: %v", missing)}
	}
	return ipcResponse{OK: true}
}

func ipcSelect(target *paneTabs, args []string) ipcResponse {
	if len(args) == 0 {
		return ipcResponse{Error: "no name given"}
	}
	fp, ok := target.ActiveFilePane()
	if !ok {
		return ipcResponse{Error: "the other pane isn't showing a file list"}
	}
	if !fp.SelectByName(args[0]) {
		return ipcResponse{Error: fmt.Sprintf("%q not found in the other pane's current listing", args[0])}
	}
	return ipcResponse{OK: true}
}

// sendIPC is the client side (cli.go's command handlers): connect, write
// one request, read one response.
func sendIPC(addr string, req ipcRequest) error {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return err
	}
	var resp ipcResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}
