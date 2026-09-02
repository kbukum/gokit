package postgres

import (
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/kbukum/gokit/database"
)

// sslModeOption is the ConnParams.Options key carrying the PostgreSQL SSL mode.
const sslModeOption = "sslmode"

// defaultPort is the standard PostgreSQL server port used when ConnParams.Port is zero.
const defaultPort = 5432

// maxPort is the largest valid TCP port number.
const maxPort = 65535

// defaultSSLMode is the secure-by-default SSL mode: it encrypts the connection and verifies the
// server certificate against a trusted CA and the requested hostname. Callers on trusted local
// networks must opt out explicitly (e.g. Options["sslmode"] = "disable").
const defaultSSLMode = "verify-full"

// DSN validates structured parameters and assembles a PostgreSQL connection string in URL form
// ("postgres://user:pass@host:port/db?sslmode=...") from them. A zero Port defaults to 5432 and an
// absent Options["sslmode"] to "verify-full"; user, password, host, path, and option values are
// URL-encoded, and IPv6 hosts are bracketed. It returns an error when Host is empty or Port is out
// of range, so invalid configuration fails fast rather than as an opaque connection error.
//
// Implementing this method makes the PostgreSQL dialect a database.StructuredDialect, so
// database.Component can build the DSN from Config.Params when no explicit Config.DSN is set.
func (dialect) DSN(p database.ConnParams) (string, error) {
	if p.Host == "" {
		return "", fmt.Errorf("postgres: host is required")
	}
	if p.Port < 0 || p.Port > maxPort {
		return "", fmt.Errorf("postgres: port %d out of range [0, %d]", p.Port, maxPort)
	}

	port := p.Port
	if port == 0 {
		port = defaultPort
	}

	query := make(url.Values, len(p.Options)+1)
	for key, value := range p.Options {
		query.Set(key, value)
	}
	if query.Get(sslModeOption) == "" {
		query.Set(sslModeOption, defaultSSLMode)
	}

	dsn := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(p.User, p.Password),
		Host:     net.JoinHostPort(p.Host, strconv.Itoa(port)),
		Path:     "/" + p.Database,
		RawQuery: query.Encode(),
	}
	return dsn.String(), nil
}

// compile-time assertion that the PostgreSQL dialect can build its DSN from structured params.
var _ database.StructuredDialect = dialect{}
