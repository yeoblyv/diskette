package sftpfs

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// AuthMethod identifies how Dial should authenticate.
type AuthMethod int

// Supported authentication methods.
const (
	// AuthPassword uses Config.Password.
	AuthPassword AuthMethod = iota
	// AuthPrivateKey reads and parses the key file at Config.KeyPath,
	// using Config.KeyPassphrase to decrypt it if it's encrypted (an
	// empty passphrase is tried first for an unencrypted key).
	AuthPrivateKey
	// AuthAgent authenticates using whatever keys are already loaded in
	// a running ssh-agent, reached via $SSH_AUTH_SOCK — a Unix-domain
	// socket, so this method only works on macOS/Linux; there is no
	// Windows support in this phase.
	AuthAgent
)

// Config describes one SFTP connection to establish. Password and
// KeyPassphrase exist only in memory for the duration of a single Dial
// call — neither is ever written to disk by this package or by anything
// that persists a Config's non-secret fields (Host/Port/Username/
// AuthMethod/KeyPath) for reconnecting later.
type Config struct {
	Host     string
	Port     int // 0 defaults to 22
	Username string

	AuthMethod AuthMethod

	// Password is used when AuthMethod is AuthPassword.
	Password string
	// KeyPath is used when AuthMethod is AuthPrivateKey.
	KeyPath string
	// KeyPassphrase is used when AuthMethod is AuthPrivateKey and the key
	// at KeyPath is encrypted.
	KeyPassphrase string
}

// addr returns host:port, defaulting Port to 22 when unset.
func (c Config) addr() string {
	port := c.Port
	if port == 0 {
		port = 22
	}
	return fmt.Sprintf("%s:%d", c.Host, port)
}

// authMethod builds the single ssh.AuthMethod matching cfg.AuthMethod.
func authMethod(cfg Config) (ssh.AuthMethod, error) {
	switch cfg.AuthMethod {
	case AuthPassword:
		return ssh.Password(cfg.Password), nil

	case AuthPrivateKey:
		keyBytes, err := os.ReadFile(cfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("reading private key: %w", err)
		}
		var signer ssh.Signer
		if cfg.KeyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(cfg.KeyPassphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(keyBytes)
		}
		if err != nil {
			return nil, fmt.Errorf("parsing private key: %w", err)
		}
		return ssh.PublicKeys(signer), nil

	case AuthAgent:
		sock := os.Getenv("SSH_AUTH_SOCK")
		if sock == "" {
			return nil, fmt.Errorf("SSH_AUTH_SOCK is not set — is ssh-agent running?")
		}
		conn, err := net.Dial("unix", sock)
		if err != nil {
			return nil, fmt.Errorf("connecting to ssh-agent: %w", err)
		}
		client := agent.NewClient(conn)
		return ssh.PublicKeysCallback(client.Signers), nil

	default:
		return nil, fmt.Errorf("sftpfs: unknown auth method %d", cfg.AuthMethod)
	}
}

// Dial establishes a new SSH connection per cfg, verifying the server's
// host key via hostKeyCallback (see VerifyHostKey), then opens an SFTP
// session over it. The returned *SFTPFS owns both and must be Close'd by
// the caller once it's no longer needed.
func Dial(ctx context.Context, cfg Config, hostKeyCallback ssh.HostKeyCallback) (*SFTPFS, error) {
	auth, err := authMethod(cfg)
	if err != nil {
		return nil, err
	}

	sshCfg := &ssh.ClientConfig{
		User:            cfg.Username,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: hostKeyCallback,
	}

	dialer := net.Dialer{}
	netConn, err := dialer.DialContext(ctx, "tcp", cfg.addr())
	if err != nil {
		return nil, err
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(netConn, cfg.addr(), sshCfg)
	if err != nil {
		return nil, err
	}
	conn := ssh.NewClient(sshConn, chans, reqs)

	client, err := sftp.NewClient(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &SFTPFS{client: client, conn: conn}, nil
}
