package ftpfs

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/jlaffaye/ftp"
)

// TLSMode identifies how (or whether) Dial protects the connection.
type TLSMode int

// Supported TLS modes.
const (
	// TLSNone is plain, unencrypted FTP — credentials and data both
	// travel in clear text. Offered for legacy/trusted-network use, not
	// the default.
	TLSNone TLSMode = iota
	// TLSExplicit issues AUTH TLS to upgrade a plain connection (RFC
	// 4217) — the modern standard, referred to as "FTPS" in the UI.
	TLSExplicit
	// TLSImplicit establishes TLS from the very first byte, historically
	// on port 990 — legacy, but still offered by some servers.
	TLSImplicit
)

// Config describes one FTP connection to establish. Password exists only
// in memory for the duration of a single Dial call — it is never written
// to disk by this package or by anything that persists a Config's
// non-secret fields (Host/Port/Username/TLSMode) for reconnecting later.
type Config struct {
	Host     string
	Port     int // 0 defaults to 21
	Username string
	Password string
	TLSMode  TLSMode

	// TLSConfig, when non-nil, replaces the default *tls.Config used for
	// TLSExplicit/TLSImplicit (ServerName: Host, full certificate
	// verification). Exists so tests can supply a RootCAs pool trusting
	// a throwaway test certificate; real callers should leave this nil
	// and get the secure default rather than opting into anything
	// weaker.
	TLSConfig *tls.Config
}

// addr returns host:port, defaulting Port to 21 when unset.
func (c Config) addr() string {
	port := c.Port
	if port == 0 {
		port = 21
	}
	return fmt.Sprintf("%s:%d", c.Host, port)
}

// Dial connects to cfg.addr(), applying whichever TLS mode was
// requested, then logs in. On success the connection is also switched to
// binary transfer mode (TYPE I) — the ASCII default would otherwise
// silently rewrite line endings in every non-text file transferred,
// corrupting it.
func Dial(ctx context.Context, cfg Config) (*FTPFS, error) {
	opts := []ftp.DialOption{ftp.DialWithContext(ctx)}

	tlsCfg := cfg.TLSConfig
	if tlsCfg == nil {
		tlsCfg = &tls.Config{ServerName: cfg.Host}
	}
	switch cfg.TLSMode {
	case TLSExplicit:
		opts = append(opts, ftp.DialWithExplicitTLS(tlsCfg))
	case TLSImplicit:
		opts = append(opts, ftp.DialWithTLS(tlsCfg))
	}

	conn, err := ftp.Dial(cfg.addr(), opts...)
	if err != nil {
		return nil, err
	}

	if err := conn.Login(cfg.Username, cfg.Password); err != nil {
		conn.Quit()
		return nil, err
	}
	if err := conn.Type(ftp.TransferTypeBinary); err != nil {
		conn.Quit()
		return nil, err
	}

	return &FTPFS{conn: conn}, nil
}
