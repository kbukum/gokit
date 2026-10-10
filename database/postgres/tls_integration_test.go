//go:build integration

package postgres_test

import (
	"context"
	"crypto/x509"
	"database/sql"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/postgres"
	pgtest "github.com/kbukum/gokit/database/postgres/testutil"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/security/tlstest"
)

func TestPostgresVerifiedPrivateCAAndEveryConnectionNamespace(t *testing.T) {
	certs := tlstest.GenerateTLSCerts(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	fixture, err := pgtest.Start(ctx, pgtest.WithTLS(pgtest.TLSFiles{CAFile: certs.CAFile, CertFile: certs.CertFile, KeyFile: certs.KeyFile}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fixture.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	db, err := database.NewWithContext(ctx, postgres.Dialect(), database.Config{Params: fixture.Params, MaxRetries: 1, LogLevel: "silent"}, logging.NewDefault("postgres-tls"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	pool, err := db.GormDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	first, err := pool.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := first.Close(); err != nil {
			t.Error(err)
		}
	}()
	second, err := pool.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := second.Close(); err != nil {
			t.Error(err)
		}
	}()
	for _, conn := range []*sql.Conn{first, second} {
		var enabled bool
		var version, path string
		if err := conn.QueryRowContext(ctx, "SELECT ssl, version FROM pg_stat_ssl WHERE pid = pg_backend_pid()").Scan(&enabled, &version); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRowContext(ctx, "SHOW search_path").Scan(&path); err != nil {
			t.Fatal(err)
		}
		if !enabled || version != "TLSv1.3" || path != `"fixture", pg_temp` {
			t.Fatal("a physical connection lost verified TLS or explicit namespace ordering")
		}
	}
	isolated, err := fixture.NewDatabase(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := isolated.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	pair, err := isolated.NewRolePair(ctx, pgtest.RolePairConfig{Schema: "app"})
	if err != nil {
		t.Fatal(err)
	}
	runtimeParams, err := pgtest.WithPasswordFile(t.TempDir(), pair.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	if runtimeParams.Options["sslmode"] != "" && runtimeParams.Options["sslmode"] != "verify-full" {
		t.Fatal("role pair did not inherit verified TLS")
	}
	runtime, err := database.NewWithContext(ctx, postgres.Dialect(), database.Config{Params: runtimeParams, MaxRetries: 1, LogLevel: "silent"}, logging.NewDefault("postgres-tls"))
	if err != nil {
		t.Fatal(err)
	}
	var verified bool
	err = runtime.GormDB.WithContext(ctx).Raw("SELECT ssl AND version = 'TLSv1.3' FROM pg_stat_ssl WHERE pid = pg_backend_pid()").Scan(&verified).Error
	if closeErr := runtime.Close(); closeErr != nil {
		t.Error(closeErr)
	}
	if err != nil || !verified {
		t.Fatalf("runtime role over password file lost TLS 1.3: %v", err)
	}
	params := fixture.Params
	params.Options = maps.Clone(params.Options)
	params.Options["sslrootcert"] = tlstest.GenerateTLSCerts(t).CAFile
	untrusted, err := database.NewWithContext(ctx, postgres.Dialect(), database.Config{Params: params, MaxRetries: 1, LogLevel: "silent"}, logging.NewDefault("postgres-tls"))
	if untrusted != nil {
		if closeErr := untrusted.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		t.Fatal("an untrusted CA opened a pool")
	}
	var unknown x509.UnknownAuthorityError
	if !errors.As(err, &unknown) {
		t.Fatalf("untrusted CA did not retain its certificate classification: %v", err)
	}
}

func TestPostgresVerifiedTLSRejectsReachableWrongHostname(t *testing.T) {
	certs := tlstest.GenerateTLSCerts(t, tlstest.WithHosts("wrong.example"))
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	fixture, err := pgtest.Start(ctx, pgtest.WithTLS(pgtest.TLSFiles{CAFile: certs.CAFile, CertFile: certs.CertFile, KeyFile: certs.KeyFile}))
	if fixture != nil {
		if closeErr := fixture.Close(context.Background()); closeErr != nil {
			t.Error(closeErr)
		}
		t.Fatal("a wrong-hostname certificate was accepted")
	}
	var hostname x509.HostnameError
	if !errors.As(err, &hostname) {
		t.Fatalf("wrong hostname did not retain its certificate classification: %v", err)
	}
}
