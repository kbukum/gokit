package testutil

import (
	"context"
	"errors"
	"maps"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/postgres"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/util"
)

// Environment variables that select a shared PostgreSQL server for [Open].
const (
	// EnvAddr is the shared server's host:port. When it is empty, Open starts an owned container.
	EnvAddr = "GOKIT_POSTGRES_TEST_ADDR"
	// EnvUser is the shared server's superuser; it defaults to "postgres".
	EnvUser = "GOKIT_POSTGRES_TEST_USER"
	// EnvPasswordFile is a file holding the superuser's password. Leave it empty for servers that trust local logins.
	EnvPasswordFile = "GOKIT_POSTGRES_TEST_PASSWORD_FILE" //nolint:gosec // Names an environment variable, not a credential.
)

// Open returns a fixture on the shared server named by [EnvAddr], or starts an owned container with [Start] when it is
// unset. A configured but unusable shared server is an error; Open never falls back to a container. TLS fixtures always
// start an owned container because they configure the server.
func Open(ctx context.Context, opts ...Option) (*Fixture, error) {
	addr := strings.TrimSpace(os.Getenv(EnvAddr))
	options := fixtureOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	if addr == "" || options.tls != nil {
		return Start(ctx, opts...)
	}
	params, err := envParams(addr)
	if err != nil {
		return nil, err
	}
	return Connect(ctx, params)
}

// Connect attaches to an existing PostgreSQL server as a superuser within a ten-second budget. The fixture owns no
// container: Close drops only the databases and roles created through it and leaves the server running, so many test
// binaries can share one server. Params must name a superuser login and an existing maintenance database.
func Connect(ctx context.Context, params database.ConnParams) (*Fixture, error) {
	if util.IsNil(ctx) {
		return nil, apperrors.InvalidInput("context", "Context is required")
	}
	if params.Host == "" || params.Port <= 0 || params.User == "" || params.Database == "" {
		return nil, apperrors.InvalidInput("params", "Shared PostgreSQL host, port, user and database are required")
	}
	params.Options = cloneOptions(params.Options)
	if params.Options["sslmode"] == "" {
		params.Options["sslmode"] = "disable"
	}
	// The pool requires an application search_path; the admin session only reads catalogs.
	if params.Options["search_path"] == "" {
		params.Options["search_path"] = "fixture"
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	log := logging.NewDefault("postgres-fixture") //nolint:contextcheck // NewDefault disables OTLP and performs no context-bearing exporter initialization.
	db, err := database.NewWithContext(ctx, postgres.Dialect(), database.Config{Params: params, MaxRetries: 1, LogLevel: "silent"}, log.WithContext(ctx))
	if err != nil {
		return nil, database.Failure(err)
	}
	var superuser bool
	checkErr := db.WithContext(ctx).Raw("SELECT rolsuper FROM pg_roles WHERE rolname = current_user").Scan(&superuser).Error
	if checkErr == nil && !superuser {
		checkErr = apperrors.InvalidInput("params", "Shared PostgreSQL login must be a superuser to isolate databases and roles")
	}
	fixture := &Fixture{Params: params}
	if checkErr == nil {
		fixture.admin, checkErr = snapshotAdmin(ctx, db)
	}
	if err := errors.Join(checkErr, db.Close()); err != nil {
		return nil, database.Failure(err)
	}
	return fixture, nil
}

func envParams(addr string) (database.ConnParams, error) {
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return database.ConnParams{}, apperrors.InvalidInput(EnvAddr, "Shared PostgreSQL address must be host:port")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 || host == "" {
		return database.ConnParams{}, apperrors.InvalidInput(EnvAddr, "Shared PostgreSQL address must be host:port")
	}
	user := strings.TrimSpace(os.Getenv(EnvUser))
	if user == "" {
		user = "postgres"
	}
	params := database.ConnParams{Host: host, Port: port, User: user, Database: "postgres", Options: map[string]string{"sslmode": "disable"}}
	if file := strings.TrimSpace(os.Getenv(EnvPasswordFile)); file != "" {
		data, err := os.ReadFile(file) //nolint:gosec // The test operator names the password file explicitly.
		if err != nil {
			return database.ConnParams{}, apperrors.InvalidInput(EnvPasswordFile, "Shared PostgreSQL password file is unreadable")
		}
		params.Password = strings.TrimRight(string(data), "\r\n")
	}
	return params, nil
}

func cloneOptions(options map[string]string) map[string]string {
	out := make(map[string]string, len(options)+1)
	maps.Copy(out, options)
	return out
}
