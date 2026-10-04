package testhost

import (
	"net"
	"net/url"
	"path/filepath"
	"strconv"

	apperrors "github.com/kbukum/gokit/errors"
)

const (
	// ProtocolVersion identifies the session contract consumed by browser fixtures.
	ProtocolVersion = "gokit.session.v1"
	// OperationPath is the fixture's protected Connect procedure.
	OperationPath = "/gokit.auth.v1.IdentityService/WhoAmI"
	// EventsPath is the fixture's protected SSE endpoint.
	EventsPath = "/events"
)

// Config identifies one explicitly owned host, its TLS files and its isolated SQLite state. Port zero requests an ephemeral listener; independent hosts need independent browser contexts because cookies are not port-isolated.
type Config struct {
	Origin    string
	StateFile string
	CertFile  string
	KeyFile   string
	RunID     string
	BuildID   string
	AssetsDir string
}

// Validate rejects cleartext, remote listeners, ambiguous origins and incomplete fixture ownership.
func (c Config) Validate() error {
	u, err := url.Parse(c.Origin)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return apperrors.InvalidInput("origin", "an exact loopback HTTPS origin is required")
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil || (host != "localhost" && host != "127.0.0.1" && host != "::1") {
		return apperrors.InvalidInput("origin", "an explicit loopback HTTPS port is required")
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 0 || number > 65535 {
		return apperrors.InvalidInput("origin", "the fixture port must be between 0 and 65535")
	}
	if c.CertFile == "" || c.KeyFile == "" {
		return apperrors.InvalidInput("tls", "certificate and private key files are required")
	}
	if !filepath.IsAbs(c.StateFile) || c.RunID == "" || c.BuildID == "" {
		return apperrors.InvalidInput("fixture", "absolute state path, run identity and build identity are required")
	}
	if c.AssetsDir != "" && !filepath.IsAbs(c.AssetsDir) {
		return apperrors.InvalidInput("assets", "the browser build directory must be absolute")
	}
	return nil
}
