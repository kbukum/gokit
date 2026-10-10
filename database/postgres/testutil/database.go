package testutil

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/xdg-go/scram"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/migration"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// Fixture operation budgets.
const (
	operationTimeout = 10 * time.Second
	scramIterations  = 4096
)

// Database is one isolated database of a Fixture. PUBLIC holds no CONNECT, TEMPORARY or public-schema CREATE right in
// it, so only role pairs created here can use it. Close drops it and its role pairs.
type Database struct {
	fixture *Fixture
	name    string
	admin   *pgx.ConnConfig

	mu     sync.Mutex
	roles  []string
	closed bool
	done   bool
	result error
}

// RolePairConfig names the application schema both roles use as their search_path.
type RolePairConfig struct {
	Schema string
}

// RolePair is a fresh least-privilege login pair: Owner may create the schema and migrate it, Runtime may only connect.
// Neither has role attributes, TEMPORARY or membership in another role. Both carry ephemeral credentials that must
// not be logged or retained. The pair is dropped with its Database.
type RolePair struct {
	Owner, Runtime database.ConnParams
}

// NewDatabase creates an isolated database within the 10-second operation budget. A database created while the fixture
// closes is rolled back.
func (f *Fixture) NewDatabase(ctx context.Context) (*Database, error) {
	if util.IsNil(ctx) {
		return nil, apperrors.InvalidInput("context", "Context is required")
	}
	admin, err := f.AdminConfig()
	if err != nil {
		return nil, err
	}
	name := "db_" + token()
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	if err := execute(ctx, admin, "CREATE DATABASE "+quote(name)); err != nil {
		return nil, database.Failure(err)
	}
	db := &Database{fixture: f, name: name, admin: admin.Copy()}
	db.admin.Database = name
	setupErr := execute(ctx, db.admin,
		"REVOKE ALL ON DATABASE "+quote(name)+" FROM PUBLIC",
		"REVOKE ALL ON SCHEMA public FROM PUBLIC",
	)
	if setupErr == nil {
		setupErr = f.track(db)
	}
	if setupErr != nil {
		return nil, errors.Join(database.Failure(setupErr), db.Close(context.WithoutCancel(ctx)))
	}
	return db, nil
}

// Name is the isolated database name.
func (d *Database) Name() string { return d.name }

// AdminConfig returns an owned superuser connection snapshot for this database, for test-only provisioning and
// assertions. It must not be logged.
func (d *Database) AdminConfig() *pgx.ConnConfig { return d.admin.Copy() }

// NewRolePair creates a fresh owner and runtime login within the 10-second budget. Passwords are random and stored as
// client-computed SCRAM-SHA-256 verifiers, so no plaintext password reaches the server.
func (d *Database) NewRolePair(ctx context.Context, cfg RolePairConfig) (RolePair, error) {
	if util.IsNil(ctx) {
		return RolePair{}, apperrors.InvalidInput("context", "Context is required")
	}
	if !migration.IsIdentifier(cfg.Schema) || cfg.Schema == "public" || strings.HasPrefix(cfg.Schema, "pg_") {
		return RolePair{}, apperrors.InvalidInput("schema", "Role pair schema must be a non-system lowercase identifier")
	}
	suffix := token()
	owner, runtime := "owner_"+suffix, "runtime_"+suffix
	ownerPassword, runtimePassword := rand.Text(), rand.Text()
	ownerVerifier, err := verifier(owner, ownerPassword)
	if err != nil {
		return RolePair{}, err
	}
	runtimeVerifier, err := verifier(runtime, runtimePassword)
	if err != nil {
		return RolePair{}, err
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return RolePair{}, apperrors.New(apperrors.ErrCodeServiceUnavailable, "PostgreSQL fixture database is closed")
	}
	// Record before creation so Close always attempts to drop them.
	d.roles = append(d.roles, runtime, owner)
	d.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	attributes := " LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS INHERIT PASSWORD "
	err = execute(ctx, d.admin,
		"CREATE ROLE "+quote(owner)+attributes+literal(ownerVerifier),
		"CREATE ROLE "+quote(runtime)+attributes+literal(runtimeVerifier),
		"GRANT CONNECT, CREATE ON DATABASE "+quote(d.name)+" TO "+quote(owner),
		"GRANT CONNECT ON DATABASE "+quote(d.name)+" TO "+quote(runtime),
	)
	if err != nil {
		return RolePair{}, database.Failure(err)
	}
	params := func(user, password string) database.ConnParams {
		p := d.fixture.Params
		p.Options = maps.Clone(p.Options)
		p.Options["search_path"] = cfg.Schema
		p.User, p.Password, p.Database = user, password, d.name
		return p
	}
	return RolePair{Owner: params(owner, ownerPassword), Runtime: params(runtime, runtimePassword)}, nil
}

// Close terminates sessions, drops the database and then its roles with a fresh 10-second budget, even after
// cancellation. A failed Close may be retried; after success, repeated calls return nil.
func (d *Database) Close(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	if d.done {
		return d.result
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), operationTimeout)
	defer cancel()
	admin := d.admin.Copy()
	admin.Database = d.fixture.Params.Database
	statements := []string{"DROP DATABASE IF EXISTS " + quote(d.name) + " WITH (FORCE)"}
	for _, role := range d.roles {
		statements = append(statements, "DROP ROLE IF EXISTS "+quote(role))
	}
	if err := execute(ctx, admin, statements...); err != nil {
		d.result = fmt.Errorf("drop PostgreSQL fixture database: %w", database.Failure(err))
		return d.result
	}
	d.done, d.result = true, nil
	d.fixture.untrack(d)
	return nil
}

func execute(ctx context.Context, config *pgx.ConnConfig, statements ...string) (err error) {
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		err = errors.Join(err, conn.Close(closeCtx))
	}()
	for _, statement := range statements {
		if _, err := conn.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

// verifier computes PostgreSQL's SCRAM-SHA-256 verifier on the client.
func verifier(user, password string) (string, error) {
	client, err := scram.SHA256.NewClient(user, password, "")
	if err != nil {
		return "", err
	}
	salt := make([]byte, 16)
	if _, err = rand.Read(salt); err != nil {
		return "", err
	}
	creds, err := client.GetStoredCredentialsWithError(scram.KeyFactors{Salt: string(salt), Iters: scramIterations})
	if err != nil {
		return "", err
	}
	encode := base64.StdEncoding.EncodeToString
	return fmt.Sprintf("SCRAM-SHA-256$%d:%s$%s:%s", scramIterations, encode(salt), encode(creds.StoredKey), encode(creds.ServerKey)), nil
}

// literal quotes a utility-statement string. Callers pass only generated base64 verifiers; role DDL cannot bind values.
func literal(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

func quote(name string) string { return pgx.Identifier{name}.Sanitize() }

// token is a fresh lowercase identifier suffix, unique across the cluster.
func token() string { return strings.ToLower(rand.Text()) }
