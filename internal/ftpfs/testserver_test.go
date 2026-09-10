package ftpfs

// A minimal, in-process FTP(S) server speaking just enough real protocol
// to drive github.com/jlaffaye/ftp (and therefore FTPFS) through its full
// vfs.FileSystem surface — the same "test against a real implementation
// of the wire protocol, not a mock of the library's Go API" approach
// internal/sftpfs's tests use with an in-process SFTP server. It serves
// files from a real local directory, so command effects (MKD, STOR, ...)
// can be verified directly against the real filesystem.

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type testFTPServer struct {
	root    string
	user    string
	pass    string
	ln      net.Listener
	tlsCfg  *tls.Config
	rootCAs *x509.CertPool // trusts tlsCfg's own self-signed leaf cert
}

// clientTLSConfig returns a *tls.Config a test client can use to dial
// this server with full certificate verification still on, trusting
// only this one throwaway self-signed cert rather than skipping
// verification altogether.
func (s *testFTPServer) clientTLSConfig() *tls.Config {
	return &tls.Config{RootCAs: s.rootCAs, ServerName: "127.0.0.1"}
}

func newTestFTPServer(t *testing.T, root, user, pass string) *testFTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	tlsCfg, rootCAs := generateTestTLSConfig(t)
	srv := &testFTPServer{root: root, user: user, pass: pass, ln: ln, tlsCfg: tlsCfg, rootCAs: rootCAs}
	go srv.serve(t)
	t.Cleanup(func() { ln.Close() })
	return srv
}

func (s *testFTPServer) addr() string { return s.ln.Addr().String() }

func (s *testFTPServer) serve(t *testing.T) {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handleConn(t, conn)
	}
}

func (s *testFTPServer) handleConn(t *testing.T, conn net.Conn) {
	sess := &ftpSession{srv: s, t: t, conn: conn, cwd: "/"}
	sess.run()
}

// ftpSession is one control connection's worth of state — its own local
// current directory (CWD), whether AUTH TLS/PROT P are active, and
// whatever's pending for a two-step command (RNFR/RNTO).
type ftpSession struct {
	srv         *testFTPServer
	t           *testing.T
	conn        net.Conn
	r           *bufio.Reader
	w           *bufio.Writer
	cwd         string
	dataTLS     bool
	renameFr    string
	pendingData net.Listener
}

func (s *ftpSession) run() {
	defer s.conn.Close()
	s.r = bufio.NewReader(s.conn)
	s.w = bufio.NewWriter(s.conn)
	s.reply(220, "test FTP server ready")

	for {
		line, err := s.r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		verb, arg, _ := strings.Cut(line, " ")
		verb = strings.ToUpper(verb)
		if !s.dispatch(verb, arg) {
			return
		}
	}
}

func (s *ftpSession) reply(code int, msg string) {
	fmt.Fprintf(s.w, "%d %s\r\n", code, msg)
	s.w.Flush()
}

func (s *ftpSession) replyMultiline(code int, lines ...string) {
	for i, l := range lines {
		if i == len(lines)-1 {
			fmt.Fprintf(s.w, "%d %s\r\n", code, l)
		} else {
			fmt.Fprintf(s.w, "%d-%s\r\n", code, l)
		}
	}
	s.w.Flush()
}

// localPath maps an FTP-visible absolute path to the real path under the
// server's root.
func (s *ftpSession) localPath(ftpPath string) string {
	if ftpPath == "" {
		ftpPath = s.cwd
	}
	if !strings.HasPrefix(ftpPath, "/") {
		ftpPath = s.cwd + "/" + ftpPath
	}
	clean := filepath.Clean(ftpPath)
	return filepath.Join(s.srv.root, clean)
}

func (s *ftpSession) dispatch(verb, arg string) bool {
	switch verb {
	case "USER":
		s.reply(331, "send password")
	case "PASS":
		if arg == s.srv.pass {
			s.reply(230, "logged in")
		} else {
			s.reply(530, "auth failed")
		}
	case "SYST":
		s.reply(215, "UNIX Type: L8")
	case "FEAT":
		s.replyMultiline(211, "Features:", " MLST type*;size*;modify*;perm*;", " MLSD", " UTF8", "End")
	case "PWD":
		s.reply(257, fmt.Sprintf("%q is the current directory", s.cwd))
	case "CWD":
		target := s.resolveDir(arg)
		if info, err := os.Stat(s.localPath(target)); err == nil && info.IsDir() {
			s.cwd = target
			s.reply(250, "directory changed")
		} else {
			s.reply(550, "no such directory")
		}
	case "CDUP":
		s.cwd = parentOf(s.cwd)
		s.reply(250, "directory changed")
	case "TYPE":
		s.reply(200, "type set")
	case "OPTS":
		s.reply(200, "options set")
	case "PASV":
		s.doPASV()
	case "EPSV":
		s.doEPSV()
	case "AUTH":
		if strings.ToUpper(arg) == "TLS" {
			s.reply(234, "AUTH TLS ok")
			tlsConn := tls.Server(s.conn, s.srv.tlsCfg)
			if err := tlsConn.Handshake(); err != nil {
				s.t.Logf("test FTP server TLS handshake failed: %v", err)
				return false
			}
			s.conn = tlsConn
			s.r = bufio.NewReader(s.conn)
			s.w = bufio.NewWriter(s.conn)
		} else {
			s.reply(504, "unsupported auth type")
		}
	case "PBSZ":
		s.reply(200, "PBSZ ok")
	case "PROT":
		if strings.ToUpper(arg) == "P" {
			s.dataTLS = true
		} else {
			s.dataTLS = false
		}
		s.reply(200, "PROT ok")
	case "MLSD":
		s.doMLSD(arg)
	case "LIST":
		s.doLIST(arg)
	case "MLST":
		s.doMLST(arg)
	case "RETR":
		s.doRETR(arg)
	case "STOR":
		s.doSTOR(arg)
	case "DELE":
		p := s.localPath(arg)
		if err := os.Remove(p); err != nil {
			s.reply(550, err.Error())
		} else {
			s.reply(250, "deleted")
		}
	case "MKD":
		p := s.localPath(arg)
		if err := os.Mkdir(p, 0o755); err != nil {
			s.reply(550, err.Error())
		} else {
			s.reply(257, fmt.Sprintf("%q created", arg))
		}
	case "RMD":
		p := s.localPath(arg)
		if err := os.Remove(p); err != nil {
			s.reply(550, err.Error())
		} else {
			s.reply(250, "removed")
		}
	case "RNFR":
		s.renameFr = arg
		s.reply(350, "ready for RNTO")
	case "RNTO":
		if err := os.Rename(s.localPath(s.renameFr), s.localPath(arg)); err != nil {
			s.reply(550, err.Error())
		} else {
			s.reply(250, "renamed")
		}
	case "SIZE":
		info, err := os.Stat(s.localPath(arg))
		if err != nil {
			s.reply(550, err.Error())
		} else {
			s.reply(213, strconv.FormatInt(info.Size(), 10))
		}
	case "MDTM":
		info, err := os.Stat(s.localPath(arg))
		if err != nil {
			s.reply(550, err.Error())
		} else {
			s.reply(213, info.ModTime().UTC().Format("20060102150405"))
		}
	case "NOOP":
		s.reply(200, "noop")
	case "QUIT":
		s.reply(221, "bye")
		return false
	default:
		s.reply(502, "not implemented")
	}
	return true
}

func (s *ftpSession) resolveDir(arg string) string {
	if arg == ".." {
		return parentOf(s.cwd)
	}
	if strings.HasPrefix(arg, "/") {
		return filepath.ToSlash(filepath.Clean(arg))
	}
	return filepath.ToSlash(filepath.Clean(s.cwd + "/" + arg))
}

func parentOf(p string) string {
	if p == "/" || p == "" {
		return "/"
	}
	parent := filepath.ToSlash(filepath.Dir(p))
	if parent == "." {
		return "/"
	}
	return parent
}

// openData opens a fresh passive-mode listener for one data transfer,
// returning it and the port to report in the PASV/EPSV reply.
func (s *ftpSession) openData() (net.Listener, int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, 0, err
	}
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	return ln, port, nil
}

func (s *ftpSession) doPASV() {
	ln, port, err := s.openData()
	if err != nil {
		s.reply(425, "can't open data connection")
		return
	}
	s.pendingData = ln
	p1, p2 := port/256, port%256
	s.reply(227, fmt.Sprintf("Entering Passive Mode (127,0,0,1,%d,%d)", p1, p2))
}

func (s *ftpSession) doEPSV() {
	ln, port, err := s.openData()
	if err != nil {
		s.reply(425, "can't open data connection")
		return
	}
	s.pendingData = ln
	s.reply(229, fmt.Sprintf("Entering Extended Passive Mode (|||%d|)", port))
}

// acceptData accepts the one data connection for the command just
// issued, wrapping it in TLS when PROT P is active.
func (s *ftpSession) acceptData() (net.Conn, error) {
	ln := s.pendingData
	s.pendingData = nil
	if ln == nil {
		return nil, fmt.Errorf("no pending data listener (PASV/EPSV not called)")
	}
	defer ln.Close()
	ln.(*net.TCPListener).SetDeadline(time.Now().Add(5 * time.Second))
	conn, err := ln.Accept()
	if err != nil {
		return nil, err
	}
	if s.dataTLS {
		tlsConn := tls.Server(conn, s.srv.tlsCfg)
		if err := tlsConn.Handshake(); err != nil {
			conn.Close()
			return nil, err
		}
		return tlsConn, nil
	}
	return conn, nil
}

func (s *ftpSession) doMLSD(arg string) {
	dir := s.localPath(arg)
	entries, err := os.ReadDir(dir)
	if err != nil {
		s.reply(550, err.Error())
		return
	}
	s.reply(150, "opening data connection")
	data, err := s.acceptData()
	if err != nil {
		s.reply(425, err.Error())
		return
	}
	for _, e := range entries {
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		fmt.Fprintf(data, "%s\r\n", mlsdFact(info))
	}
	data.Close()
	s.reply(226, "transfer complete")
}

func mlsdFact(info os.FileInfo) string {
	typ := "file"
	if info.IsDir() {
		typ = "dir"
	}
	return fmt.Sprintf("type=%s;size=%d;modify=%s; %s", typ, info.Size(), info.ModTime().UTC().Format("20060102150405"), info.Name())
}

func (s *ftpSession) doLIST(arg string) {
	dir := s.localPath(arg)
	entries, err := os.ReadDir(dir)
	if err != nil {
		s.reply(550, err.Error())
		return
	}
	s.reply(150, "opening data connection")
	data, err := s.acceptData()
	if err != nil {
		s.reply(425, err.Error())
		return
	}
	for _, e := range entries {
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		perm := "-rw-r--r--"
		if info.IsDir() {
			perm = "drwxr-xr-x"
		}
		fmt.Fprintf(data, "%s 1 owner group %d Jan 1 00:00 %s\r\n", perm, info.Size(), info.Name())
	}
	data.Close()
	s.reply(226, "transfer complete")
}

func (s *ftpSession) doMLST(arg string) {
	p := s.localPath(arg)
	info, err := os.Stat(p)
	if err != nil {
		s.reply(550, err.Error())
		return
	}
	name := arg
	if name == "" {
		name = s.cwd
	}
	s.replyMultiline(250, "Listing "+name, " "+mlsdFact(info), "End")
}

func (s *ftpSession) doRETR(arg string) {
	p := s.localPath(arg)
	f, err := os.Open(p)
	if err != nil {
		s.reply(550, err.Error())
		return
	}
	defer f.Close()
	s.reply(150, "opening data connection")
	data, err := s.acceptData()
	if err != nil {
		s.reply(425, err.Error())
		return
	}
	io.Copy(data, f)
	data.Close()
	s.reply(226, "transfer complete")
}

func (s *ftpSession) doSTOR(arg string) {
	p := s.localPath(arg)
	s.reply(150, "opening data connection")
	data, err := s.acceptData()
	if err != nil {
		s.reply(425, err.Error())
		return
	}
	f, ferr := os.Create(p)
	if ferr != nil {
		data.Close()
		s.reply(550, ferr.Error())
		return
	}
	io.Copy(f, data)
	f.Close()
	data.Close()
	s.reply(226, "transfer complete")
}

// generateTestTLSConfig creates a throwaway self-signed cert for the test
// server's AUTH TLS support, and a CertPool trusting that same cert for
// the test client side — so tests exercise real certificate verification
// (just against a one-off trusted cert) rather than disabling it.
func generateTestTLSConfig(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"127.0.0.1"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	cert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return &tls.Config{Certificates: []tls.Certificate{cert}}, pool
}
