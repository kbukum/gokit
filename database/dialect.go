package database

import (
	"fmt"

	"github.com/kbukum/gokit/provider/namedregistry"

	"gorm.io/gorm"
)

// Dialect is a database backend: it names itself and opens a GORM connection for a DSN.
// Adapters register a Dialect with a [DialectRegistry] or pass one to [Component.WithDialect];
// core stays driver-agnostic and never imports a driver SDK — the adapter owns that dependency.
type Dialect interface {
	// Name reports the backend identifier, e.g. "postgres".
	Name() string
	// Open returns the GORM dialector for the given DSN.
	Open(dsn string) gorm.Dialector
}

// StructuredDialect is the opt-in capability for backends that can assemble their own
// driver-specific DSN from agnostic [ConnParams]. Backends whose connection string has no
// structured form (such as SQLite's file path) implement only [Dialect]; callers set
// [Config.DSN] directly for those.
type StructuredDialect interface {
	Dialect
	// DSN validates the structured connection parameters and serializes them into this backend's
	// DSN format. It returns an error for invalid input (e.g. an out-of-range port or a missing
	// required field) so bad configuration fails fast at the trust boundary rather than surfacing
	// as an opaque connection failure after retries.
	DSN(ConnParams) (string, error)
}

// ConnParams are driver-agnostic, structured connection parameters. A [StructuredDialect]
// serializes them into its own DSN format; core never interprets them. Backend-specific knobs
// (sslmode, tls, encrypt, charset, …) live in Options so the common shape stays shared while
// each dialect reads only the keys it understands.
type ConnParams struct {
	// Host is the database server hostname or IP.
	Host string `yaml:"host" mapstructure:"host"`
	// Port is the database server port. A dialect supplies its own default when zero.
	Port int `yaml:"port" mapstructure:"port"`
	// User is the database user.
	User string `yaml:"user" mapstructure:"user"`
	// Password is the database password (from env var, not committed).
	Password string `yaml:"password" mapstructure:"password"`
	// Database is the database (or service) name.
	Database string `yaml:"database" mapstructure:"database"`
	// Options carries backend-specific parameters keyed by the dialect's own vocabulary.
	Options map[string]string `yaml:"options" mapstructure:"options"`
}

// IsZero reports whether no connection parameters are set.
func (p ConnParams) IsZero() bool {
	return p.Host == "" && p.Port == 0 && p.User == "" &&
		p.Password == "" && p.Database == "" && len(p.Options) == 0
}

// DialectRegistry stores database dialects by backend name without package-level global state.
// Register several dialects in one registry and select the backend by name through configuration.
type DialectRegistry struct {
	inner *namedregistry.Registry[Dialect]
}

// NewDialectRegistry creates an isolated dialect registry.
func NewDialectRegistry() *DialectRegistry {
	return &DialectRegistry{inner: namedregistry.New[Dialect]("database")}
}

// Register stores a dialect under its [Dialect.Name].
func (r *DialectRegistry) Register(d Dialect) error {
	if d == nil {
		return fmt.Errorf("database: cannot register a nil dialect")
	}
	return r.inner.Register(d.Name(), d)
}

// Get returns a dialect by backend name.
func (r *DialectRegistry) Get(name string) (Dialect, bool) {
	return r.inner.Get(name)
}
