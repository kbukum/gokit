package database

import (
	"context"
	"encoding/json"
	"fmt"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/provider/namedregistry"
	"github.com/kbukum/gokit/util"

	"gorm.io/gorm"
)

// Dialect prepares a validated backend-specific connection factory without opening a pool.
// Adapters register a Dialect with a [DialectRegistry] or pass one to [Component.WithDialect];
// core stays driver-agnostic and never imports a driver SDK — the adapter owns that dependency.
type Dialect interface {
	// Name reports the backend identifier, e.g. "postgres".
	Name() string
	Prepare(context.Context, ConnectionInput) (Opener, error)
}

// Opener creates a fresh owned dialector/pool for each connection attempt. A failed Open releases anything it allocated.
type Opener interface {
	Open(context.Context) (gorm.Dialector, error)
}

// ConnectionInput selects exactly one opaque or structured backend configuration.
type ConnectionInput struct {
	DSN     string
	Params  ConnParams
	Secrets SecretSource `json:"-"`
}

func (i ConnectionInput) Validate() error {
	switch {
	case len(i.DSN) > 8192 || len(i.Params.Options) > 32:
		return apperrors.InvalidInput("connection", "Connection configuration exceeds its bounds")
	case i.DSN != "" && !i.Params.IsZero():
		return apperrors.InvalidInput("connection", "DSN and structured parameters are mutually exclusive")
	case i.DSN == "" && i.Params.IsZero():
		return apperrors.InvalidInput("connection", "DSN or structured parameters are required")
	case i.Params.Password != "" && i.Params.PasswordFile != "":
		return apperrors.InvalidInput("password", "Password and password_file are mutually exclusive")
	}
	return nil
}

// ConnParams are driver-agnostic structured connection parameters, never a credential URL. Backend-specific knobs
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
	// PasswordFile names a bounded regular-file secret, mutually exclusive with Password.
	PasswordFile string `yaml:"password_file" mapstructure:"password_file"`
	// Database is the database (or service) name.
	Database string `yaml:"database" mapstructure:"database"`
	// Options carries backend-specific parameters keyed by the dialect's own vocabulary.
	Options map[string]string `yaml:"options" mapstructure:"options"`
}

// IsZero reports whether no connection parameters are set.
func (p ConnParams) IsZero() bool {
	return p.Host == "" && p.Port == 0 && p.User == "" &&
		p.Password == "" && p.PasswordFile == "" && p.Database == "" && len(p.Options) == 0
}

func (ConnParams) String() string     { return "Connection parameters (credentials redacted)" }
func (p ConnParams) GoString() string { return p.String() }

func (p ConnParams) MarshalJSON() ([]byte, error) {
	type fields ConnParams
	return json.Marshal(struct {
		fields
		Password util.SecretString
	}{fields: fields(p), Password: util.NewSecretString(p.Password)})
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
	if util.IsNil(d) {
		return fmt.Errorf("database: cannot register a nil dialect")
	}
	return r.inner.Register(d.Name(), d)
}

// Get returns a dialect by backend name.
func (r *DialectRegistry) Get(name string) (Dialect, bool) {
	return r.inner.Get(name)
}
