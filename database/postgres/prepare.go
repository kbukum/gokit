package postgres

import (
	"context"
	"crypto/tls"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/migration"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/util"
)

// Prepare resolves explicit credentials and builds a parser-owned pgx configuration without allocating connections. PostgreSQL opaque input is a credential-free URL; structured parameters carry passwords.
func Prepare(ctx context.Context, input database.ConnectionInput) (database.Opener, error) {
	if util.IsNil(ctx) {
		return nil, apperrors.InvalidInput("context", "Context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}
	if err := rejectAmbient(); err != nil {
		return nil, err
	}
	params := input.Params
	if input.DSN != "" {
		var err error
		params, err = opaqueParams(input.DSN)
		if err != nil {
			return nil, err
		}
	}
	if params.Host == "" || len(params.Host) > 255 || strings.ContainsAny(params.Host, "/,\x00 \t\r\n") {
		return nil, apperrors.InvalidInput("host", "One TCP database host is required")
	}
	if params.Port < 0 || params.Port > 65535 {
		return nil, apperrors.InvalidInput("port", "Database port is out of range")
	}
	if params.User == "" || params.Database == "" || len(params.User) > 63 || len(params.Database) > 63 ||
		strings.ContainsRune(params.User+params.Database, '\x00') {
		return nil, apperrors.InvalidInput("connection", "Explicit bounded user and database names are required")
	}
	runtime, tlsConfig, err := connectionOptions(params)
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(runtime["timezone"])
	if err != nil {
		return nil, apperrors.InvalidInput("timezone", "Unsupported timezone").WithCause(database.Failure(err))
	}
	secret, err := database.ResolvePassword(ctx, params, input.Secrets)
	if err != nil {
		return nil, err
	}
	if ambientErr := rejectAmbient(); ambientErr != nil {
		return nil, ambientErr
	}
	config, err := pgx.ParseConfigWithOptions(
		"host=127.0.0.1 user=gokit dbname=gokit password=explicit sslmode=disable",
		pgx.ParseConfigOptions{ParseConfigOptions: pgconn.ParseConfigOptions{
			ConnStringAllowedKeys: []string{"host", "user", "dbname", "password", "sslmode"},
		}},
	)
	if err != nil {
		return nil, database.Failure(err)
	}
	config.Host, config.User, config.Database = params.Host, params.User, params.Database
	config.Port = uint16(params.Port)
	if config.Port == 0 {
		config.Port = 5432
	}
	config.Password = secret.Expose()
	config.TLSConfig, config.Fallbacks = tlsConfig, nil
	config.RuntimeParams = runtime
	config.ConnectTimeout = 5 * time.Second
	return &opener{config: config, location: location}, nil
}

func rejectAmbient() error {
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "PG") && value != "" {
			return apperrors.InvalidInput("environment", "Ambient PostgreSQL configuration is unsupported; configure the adapter explicitly")
		}
	}
	return nil
}

func opaqueParams(dsn string) (database.ConnParams, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return database.ConnParams{}, apperrors.InvalidInput("dsn", "Invalid PostgreSQL URL").WithCause(database.Failure(err))
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" || u.User == nil || u.Fragment != "" {
		return database.ConnParams{}, apperrors.InvalidInput("dsn", "PostgreSQL opaque input must be a credential-free PostgreSQL URL")
	}
	if _, present := u.User.Password(); present {
		return database.ConnParams{}, apperrors.InvalidInput("dsn", "Password-bearing URLs are unsupported; use structured credentials")
	}
	port := 0
	if u.Port() != "" {
		port, err = strconv.Atoi(u.Port())
		if err != nil {
			return database.ConnParams{}, apperrors.InvalidInput("port", "Invalid PostgreSQL port")
		}
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return database.ConnParams{}, apperrors.InvalidInput("options", "Invalid PostgreSQL URL options").WithCause(database.Failure(err))
	}
	params := database.ConnParams{Host: u.Hostname(), Port: port, User: u.User.Username(), Database: strings.TrimPrefix(u.Path, "/"), Options: make(map[string]string, len(query))}
	for key, values := range query {
		if len(values) != 1 {
			return database.ConnParams{}, apperrors.InvalidInput("options", "Repeated PostgreSQL options are unsupported")
		}
		params.Options[key] = values[0]
	}
	return params, nil
}

func connectionOptions(params database.ConnParams) (map[string]string, *tls.Config, error) {
	for key, value := range params.Options {
		switch key {
		case "sslmode", "sslrootcert", "application_name", "search_path", "statement_timeout", "idle_in_transaction_session_timeout", "timezone":
		default:
			return nil, nil, apperrors.InvalidInput("options", "Unsupported PostgreSQL option")
		}
		if len(value) > 4096 || strings.ContainsRune(value, '\x00') {
			return nil, nil, apperrors.InvalidInput("options", "PostgreSQL option exceeds its encoding bounds")
		}
	}
	schema := params.Options["search_path"]
	if !validSchema(schema) {
		return nil, nil, apperrors.InvalidInput("search_path", "One non-public, non-system application schema is required")
	}
	runtime := map[string]string{
		"search_path":      pgx.Identifier{schema}.Sanitize() + ", pg_temp",
		"application_name": "gokit", "statement_timeout": "30000",
		"idle_in_transaction_session_timeout": "10000", "timezone": "UTC",
	}
	for _, key := range []string{"statement_timeout", "idle_in_transaction_session_timeout"} {
		if value := params.Options[key]; value != "" {
			ms, err := strconv.ParseInt(value, 10, 32)
			if err != nil || ms < 1 || ms > 300000 {
				return nil, nil, apperrors.InvalidInput(key, "Session timeout must contain 1 to 300000 milliseconds")
			}
			runtime[key] = value
		}
	}
	if name := params.Options["application_name"]; name != "" {
		if len(name) > 63 {
			return nil, nil, apperrors.InvalidInput("application_name", "Application name exceeds 63 bytes")
		}
		runtime["application_name"] = name
	}
	if zone := params.Options["timezone"]; zone != "" {
		runtime["timezone"] = zone
	}
	mode := params.Options["sslmode"]
	if mode == "disable" {
		if params.Options["sslrootcert"] != "" {
			return nil, nil, apperrors.InvalidInput("sslrootcert", "A root certificate requires verified TLS")
		}
		return runtime, nil, nil
	}
	if mode != "" && mode != "verify-full" {
		return nil, nil, apperrors.InvalidInput("sslmode", "SSL mode must be verify-full or explicit disable")
	}
	tlsConfig, err := (&security.TLSConfig{
		ServerName: params.Host, CAFile: params.Options["sslrootcert"], MinVersion: tls.VersionTLS13,
	}).Build()
	if err != nil {
		return nil, nil, database.Failure(err)
	}
	return runtime, tlsConfig, nil
}

// validSchema applies the shared identifier grammar plus PostgreSQL's reserved schemas.
func validSchema(name string) bool {
	return migration.IsIdentifier(name) && name != "public" && !strings.HasPrefix(name, "pg_")
}

var _ database.Dialect = dialect{}
