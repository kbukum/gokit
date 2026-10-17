//go:build integration

package testutil_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	pgtest "github.com/kbukum/gokit/database/postgres/testutil"
)

func TestConnectedFixtureCleansOnlyItsOwnDatabases(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	server, err := pgtest.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	neighbor, err := server.NewDatabase(ctx)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := pgtest.Connect(ctx, server.Params)
	if err != nil {
		t.Fatal(err)
	}
	if shared.ContainerID() != "" {
		t.Fatal("connected fixture claims container ownership")
	}
	db, err := shared.NewDatabase(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := db.NewRolePair(ctx, pgtest.RolePairConfig{Schema: "app"})
	if err != nil {
		t.Fatal(err)
	}
	runtime := pair.Runtime
	conn, err := pgx.Connect(ctx, "host="+runtime.Host+" port="+strconv.Itoa(runtime.Port)+" user="+runtime.User+
		" password="+runtime.Password+" dbname="+runtime.Database+" sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(ctx); err != nil {
		t.Fatal(err)
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	for range 2 {
		if err := shared.Close(canceled); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := shared.NewDatabase(ctx); err == nil {
		t.Fatal("closed connected fixture created a database")
	}
	admin, err := server.AdminConfig()
	if err != nil {
		t.Fatal("shared server closed with a connected fixture: ", err)
	}
	check, err := pgx.ConnectConfig(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = check.Close(context.Background()) }()
	var dbs, roles int
	if err := check.QueryRow(ctx, "SELECT count(*) FROM pg_database WHERE datname = $1", db.Name()).Scan(&dbs); err != nil || dbs != 0 {
		t.Fatalf("connected fixture left its database: %d %v", dbs, err)
	}
	if err := check.QueryRow(ctx, "SELECT count(*) FROM pg_roles WHERE rolname = ANY($1)", []string{pair.Owner.User, pair.Runtime.User}).Scan(&roles); err != nil || roles != 0 {
		t.Fatalf("connected fixture left its roles: %d %v", roles, err)
	}
	if err := check.QueryRow(ctx, "SELECT count(*) FROM pg_database WHERE datname = $1", neighbor.Name()).Scan(&dbs); err != nil || dbs != 1 {
		t.Fatalf("connected fixture dropped a database it does not own: %d %v", dbs, err)
	}
}

func TestConnectRejectsNonSuperuser(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	server, err := pgtest.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	db, err := server.NewDatabase(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := db.NewRolePair(ctx, pgtest.RolePairConfig{Schema: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if fixture, err := pgtest.Connect(ctx, pair.Owner); err == nil {
		_ = fixture.Close(ctx)
		t.Fatal("connected with a role that cannot isolate databases")
	}
}

func TestOpenAttachesToTheConfiguredSharedServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	server, err := pgtest.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	file := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(file, []byte(server.Params.Password+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(pgtest.EnvAddr, server.Params.Host+":"+strconv.Itoa(server.Params.Port))
	t.Setenv(pgtest.EnvUser, server.Params.User)
	t.Setenv(pgtest.EnvPasswordFile, file)
	shared, err := pgtest.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if shared.ContainerID() != "" {
		t.Fatal("Open started a container despite a configured shared server")
	}
	db, err := shared.NewDatabase(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.NewRolePair(ctx, pgtest.RolePairConfig{Schema: "app"}); err != nil {
		t.Fatal(err)
	}
	if err := shared.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
