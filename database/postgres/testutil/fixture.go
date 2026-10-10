package testutil

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moby/moby/client"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/postgres"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/util"
)

// Image is the immutable multi-architecture PostgreSQL 17 integration image.
const Image = "postgres:17-alpine@sha256:18cfe3ef5e6815560c98237d6216d1e5119702fb0f3894c8785dd58b8bbe5d73"

// Fixture owns one isolated PostgreSQL container. Params carry ephemeral credentials and must not be logged or retained as evidence.
type Fixture struct {
	Params    database.ConnParams
	admin     *pgx.ConnConfig
	terminate func(context.Context) error
	mu        sync.Mutex
	closed    bool
	databases map[*Database]struct{}
}

// Start requires a reachable Docker daemon, bounds provisioning to three minutes, and releases any partial container on failure. Each invocation owns a separate container and database.
func Start(ctx context.Context, opts ...Option) (*Fixture, error) {
	if util.IsNil(ctx) {
		return nil, apperrors.InvalidInput("context", "Context is required")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	options := fixtureOptions{}
	for _, opt := range opts {
		if opt == nil {
			return nil, apperrors.InvalidInput("option", "Fixture option is required")
		}
		opt(&options)
	}
	customizers := []testcontainers.ContainerCustomizer{
		tcpostgres.WithDatabase("fixture"),
		tcpostgres.WithUsername("fixture"),
		tcpostgres.BasicWaitStrategies(),
		tcpostgres.WithSQLDriver("pgx"),
	}
	if files := options.tls; files != nil {
		if files.CAFile == "" || files.CertFile == "" || files.KeyFile == "" {
			return nil, apperrors.InvalidInput("tls", "Fixture CA, certificate and key files are required")
		}
		if _, err := (&security.TLSConfig{CAFile: files.CAFile, CertFile: files.CertFile, KeyFile: files.KeyFile, ServerName: "localhost", MinVersion: tls.VersionTLS13}).Build(); err != nil {
			return nil, database.Failure(err)
		}
		customizers = append(customizers,
			tcpostgres.WithSSLCert(files.CAFile, files.CertFile, files.KeyFile),
			testcontainers.WithCmd("postgres", "-c", "ssl=on", "-c", "ssl_min_protocol_version=TLSv1.3",
				"-c", "ssl_ca_file=/tmp/testcontainers-go/postgres/ca_cert.pem",
				"-c", "ssl_cert_file=/tmp/testcontainers-go/postgres/server.cert",
				"-c", "ssl_key_file=/tmp/testcontainers-go/postgres/server.key"),
		)
	}
	if err := DockerReady(ctx); err != nil {
		return nil, err
	}
	password := rand.Text()
	customizers = append(customizers, tcpostgres.WithPassword(password))
	container, err := tcpostgres.Run(ctx, Image, customizers...)
	fixture := &Fixture{}
	if container != nil {
		fixture.terminate = func(cleanupCtx context.Context) error { return container.Terminate(cleanupCtx) }
	}
	if err != nil {
		return nil, errors.Join(fmt.Errorf("provision PostgreSQL fixture: %w", database.Failure(err)), fixture.Close(ctx))
	}
	host, err := container.Host(ctx)
	if err != nil {
		return nil, errors.Join(database.Failure(err), fixture.Close(ctx))
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return nil, errors.Join(database.Failure(err), fixture.Close(ctx))
	}
	fixture.Params = database.ConnParams{Host: host, Port: int(port.Num()), User: "fixture", Password: password, Database: "fixture", Options: map[string]string{"sslmode": "disable", "search_path": "fixture"}}
	if options.tls != nil {
		fixture.Params.Options["sslmode"] = "verify-full"
		fixture.Params.Options["sslrootcert"] = options.tls.CAFile
	}
	setupCtx, setupCancel := context.WithTimeout(ctx, 10*time.Second)
	defer setupCancel()
	log := logging.NewDefault("postgres-fixture") //nolint:contextcheck // NewDefault disables OTLP and performs no context-bearing exporter initialization.
	db, err := database.NewWithContext(setupCtx, postgres.Dialect(), database.Config{Params: fixture.Params, MaxRetries: 1, LogLevel: "silent"}, log.WithContext(setupCtx))
	if err != nil {
		return nil, errors.Join(database.Failure(err), fixture.Close(ctx))
	}
	setupErr := db.WithContext(setupCtx).Exec("CREATE SCHEMA fixture AUTHORIZATION fixture").Error
	var snapshotErr error
	if setupErr == nil {
		fixture.admin, snapshotErr = snapshotAdmin(setupCtx, db)
	}
	if err := errors.Join(setupErr, snapshotErr, db.Close()); err != nil {
		return nil, errors.Join(database.Failure(err), fixture.Close(ctx))
	}
	return fixture, nil
}

// DockerReady checks the declared Docker prerequisite with a five-second ceiling. It returns a setup error, never a skipped or successful gate.
func DockerReady(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	docker, err := client.New(client.FromEnv)
	if err != nil {
		return fmt.Errorf("docker prerequisite: %w", err)
	}
	_, pingErr := docker.Ping(ctx, client.PingOptions{})
	if err := errors.Join(pingErr, docker.Close()); err != nil {
		return fmt.Errorf("docker prerequisite: %w", err)
	}
	return nil
}

// Close closes every open Database, then terminates the owned container using a fresh thirty-second budget even when
// the caller is canceled. Every cleanup failure is returned. A failed termination is retried by the next Close; after
// termination succeeds, repeated calls return nil.
func (f *Fixture) Close(ctx context.Context) error {
	f.mu.Lock()
	f.closed = true
	children := make([]*Database, 0, len(f.databases))
	for child := range f.databases {
		children = append(children, child)
	}
	f.mu.Unlock()
	var childErr error
	for _, child := range children {
		childErr = errors.Join(childErr, child.Close(ctx))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.terminate == nil {
		return childErr
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := f.terminate(ctx); err != nil {
		return errors.Join(childErr, fmt.Errorf("terminate PostgreSQL fixture: %w", err))
	}
	f.terminate = nil
	clear(f.databases)
	return childErr
}

func (f *Fixture) track(db *Database) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return apperrors.New(apperrors.ErrCodeServiceUnavailable, "PostgreSQL fixture is closed")
	}
	if f.databases == nil {
		f.databases = map[*Database]struct{}{}
	}
	f.databases[db] = struct{}{}
	return nil
}

func (f *Fixture) untrack(db *Database) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.databases, db)
}
