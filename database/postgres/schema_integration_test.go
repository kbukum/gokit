//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/migration"
	"github.com/kbukum/gokit/database/postgres"
	pgtest "github.com/kbukum/gokit/database/postgres/testutil"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
)

var auditSet = fstest.MapFS{
	"1_audit.up.sql": {Data: []byte(`
		CREATE TABLE audit_events (id SERIAL PRIMARY KEY, note TEXT NOT NULL);
		CREATE TABLE secrets (id INTEGER PRIMARY KEY, value TEXT NOT NULL);
		INSERT INTO secrets VALUES (1, 'hidden');
		CREATE FUNCTION reveal() RETURNS TEXT LANGUAGE sql SECURITY DEFINER SET search_path = pg_catalog AS $$ SELECT 'x' $$;`)},
}

func schemaPlan(role string) postgres.SchemaPlan {
	return postgres.SchemaPlan{
		Name: "app",
		Sets: []migration.Set{
			{FS: migrationsFS, Path: "testdata/migrations", Table: migration.Table{Schema: "app", Name: "widget_schema_migrations"}, ExpectedVersion: 2},
			{FS: auditSet, Path: ".", Table: migration.Table{Schema: "app", Name: "audit_schema_migrations"}, ExpectedVersion: 1},
		},
		RuntimeAccess: postgres.RuntimeAccess{
			Role: role,
			Relations: []postgres.RelationAccess{
				{Name: "widgets", Privileges: []postgres.Privilege{postgres.Select, postgres.Insert, postgres.Update, postgres.Delete}},
				{Name: "audit_events", Privileges: []postgres.Privilege{postgres.Select, postgres.Insert}},
			},
			Sequences: []string{"widgets_id_seq", "audit_events_id_seq"},
		},
	}
}

type schemaEnv struct {
	db             *pgtest.Database
	pair           pgtest.RolePair
	owner, runtime *database.DB
	admin          *pgx.ConnConfig
}

func open(t *testing.T, ctx context.Context, params database.ConnParams) *database.DB {
	t.Helper()
	db, err := database.NewWithContext(ctx, postgres.Dialect(), database.Config{Params: params, MaxRetries: 1, LogLevel: "silent"}, logging.NewDefault("test"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

func newSchemaEnv(t *testing.T, ctx context.Context, fixture *pgtest.Fixture) schemaEnv {
	t.Helper()
	db, err := fixture.NewDatabase(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	pair, err := db.NewRolePair(ctx, pgtest.RolePairConfig{Schema: "app"})
	if err != nil {
		t.Fatal(err)
	}
	env := schemaEnv{db: db, pair: pair, admin: db.AdminConfig()}
	env.owner = open(t, ctx, pair.Owner)
	pool, err := env.owner.GormDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	env.runtime = open(t, ctx, pair.Runtime)
	return env
}

func exec(t *testing.T, ctx context.Context, config *pgx.ConnConfig, statements ...string) {
	t.Helper()
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	for _, statement := range statements {
		if _, err := conn.Exec(ctx, statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

func accessDenied(err error) bool {
	return err != nil && strings.Contains(err.Error(), "database access is not ready")
}

func asOwner(env schemaEnv) *pgx.ConnConfig {
	config := env.admin.Copy()
	config.User, config.Password = env.pair.Owner.User, env.pair.Owner.Password
	return config
}

func schemaExists(t *testing.T, ctx context.Context, env schemaEnv) bool {
	t.Helper()
	var exists bool
	if err := env.owner.WithContext(ctx).Raw("SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'app')").Scan(&exists).Error; err != nil {
		t.Fatal(err)
	}
	return exists
}

func TestSchemaPlanLeastPrivilege(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	fixture, err := pgtest.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fixture.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})

	t.Run("applies exact runtime access on one connection", func(t *testing.T) {
		env := newSchemaEnv(t, ctx, fixture)
		plan := schemaPlan(env.pair.Runtime.User)
		for range 2 {
			if err := postgres.ApplySchema(ctx, env.owner.GormDB, plan); err != nil {
				t.Fatal(err)
			}
		}
		if err := postgres.ReadyAccess(ctx, env.runtime.GormDB, plan); err != nil {
			t.Fatal(err)
		}
		if err := postgres.ReadyAccess(ctx, env.owner.GormDB, plan); !accessDenied(err) {
			t.Fatal("the migration role passed runtime readiness")
		}
		run := env.runtime.WithContext(ctx)
		for _, allowed := range []string{
			"INSERT INTO widgets(name) VALUES ('a')",
			"UPDATE widgets SET price = 2",
			"INSERT INTO audit_events(note) VALUES ('n')",
			"SELECT version FROM audit_schema_migrations",
			"DELETE FROM widgets",
		} {
			if err := run.Exec(allowed).Error; err != nil {
				t.Fatalf("%s: %v", allowed, err)
			}
		}
		for _, denied := range []string{
			"CREATE TABLE app.extra(id INTEGER)",
			"CREATE TABLE public.extra(id INTEGER)",
			"CREATE TEMP TABLE scratch(id INTEGER)",
			"UPDATE widget_schema_migrations SET dirty = true",
			"DELETE FROM audit_events",
			"TRUNCATE widgets",
			"SELECT value FROM secrets",
			"SELECT reveal()",
		} {
			if err := run.Exec(denied).Error; err == nil {
				t.Fatalf("runtime role was allowed: %s", denied)
			}
		}
		for _, cfg := range []migration.Config{{Table: plan.Sets[0].Table}, {Table: plan.Sets[1].Table}} {
			cfg.DB, cfg.Driver = env.runtime.GormDB, postgres.MigrateDriver()
			if err := cfg.Ready(ctx, map[string]uint{"widget_schema_migrations": 2, "audit_schema_migrations": 1}[cfg.Table.Name]); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("serializes concurrent appliers", func(t *testing.T) {
		env := newSchemaEnv(t, ctx, fixture)
		second := open(t, ctx, env.pair.Owner)
		plan := schemaPlan(env.pair.Runtime.User)
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i, db := range []*database.DB{env.owner, second} {
			wg.Go(func() { errs[i] = postgres.ApplySchema(ctx, db.GormDB, plan) })
		}
		wg.Wait()
		if err := errors.Join(errs...); err != nil {
			t.Fatal(err)
		}
		if err := postgres.ReadyAccess(ctx, env.runtime.GormDB, plan); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("reconciles drifted direct and PUBLIC grants", func(t *testing.T) {
		env := newSchemaEnv(t, ctx, fixture)
		plan := schemaPlan(env.pair.Runtime.User)
		if err := postgres.ApplySchema(ctx, env.owner.GormDB, plan); err != nil {
			t.Fatal(err)
		}
		for _, drift := range []string{
			"GRANT TRUNCATE ON app.widgets TO " + env.pair.Runtime.User,
			"GRANT DELETE ON app.audit_events TO PUBLIC",
			"GRANT UPDATE (note) ON app.audit_events TO " + env.pair.Runtime.User,
			"GRANT SELECT ON app.audit_events TO " + env.pair.Runtime.User + " WITH GRANT OPTION",
			"GRANT INSERT ON app.widget_schema_migrations TO " + env.pair.Runtime.User,
			"GRANT MAINTAIN ON app.widgets TO " + env.pair.Runtime.User,
			"GRANT MAINTAIN ON app.widgets TO PUBLIC",
		} {
			exec(t, ctx, asOwner(env), drift)
			if err := postgres.ReadyAccess(ctx, env.runtime.GormDB, plan); !accessDenied(err) {
				t.Fatalf("readiness accepted drift %s: %v", drift, err)
			}
			if err := postgres.ApplySchema(ctx, env.owner.GormDB, plan); err != nil {
				t.Fatalf("reconcile %s: %v", drift, err)
			}
			if err := postgres.ReadyAccess(ctx, env.runtime.GormDB, plan); err != nil {
				t.Fatalf("after reconciling %s: %v", drift, err)
			}
		}
		exec(t, ctx, asOwner(env), "GRANT SELECT ON app.secrets TO PUBLIC")
		if err := postgres.ApplySchema(ctx, env.owner.GormDB, plan); err != nil {
			t.Fatal(err)
		}
		if err := env.runtime.WithContext(ctx).Exec("SELECT value FROM secrets").Error; err == nil {
			t.Fatal("undeclared PUBLIC grant survived reconciliation")
		}
	})

	t.Run("rejects inherited and assumable privilege", func(t *testing.T) {
		env := newSchemaEnv(t, ctx, fixture)
		plan := schemaPlan(env.pair.Runtime.User)
		if err := postgres.ApplySchema(ctx, env.owner.GormDB, plan); err != nil {
			t.Fatal(err)
		}
		helper := "helper_" + strings.TrimPrefix(env.pair.Runtime.User, "runtime_")
		exec(t, ctx, env.admin, "CREATE ROLE "+helper+" NOLOGIN")
		t.Cleanup(func() { exec(t, context.Background(), env.admin, "DROP OWNED BY "+helper, "DROP ROLE "+helper) })
		for _, bypass := range [][]string{
			{"GRANT SELECT ON app.secrets TO " + helper, "GRANT " + helper + " TO " + env.pair.Runtime.User},
			{"GRANT EXECUTE ON FUNCTION app.reveal() TO " + helper, "GRANT " + helper + " TO " + env.pair.Runtime.User},
			{"GRANT SELECT ON app.secrets TO " + helper, "GRANT " + helper + " TO " + env.pair.Runtime.User + " WITH INHERIT FALSE, SET TRUE"},
			{"GRANT EXECUTE ON FUNCTION app.reveal() TO " + helper, "GRANT " + helper + " TO " + env.pair.Runtime.User + " WITH INHERIT FALSE, SET TRUE"},
			{"GRANT MAINTAIN ON app.secrets TO " + helper, "GRANT " + helper + " TO " + env.pair.Runtime.User},
			{"GRANT " + helper + " TO " + env.pair.Runtime.User + " WITH INHERIT FALSE, SET FALSE"},
			{"GRANT " + env.pair.Owner.User + " TO " + env.pair.Runtime.User + " WITH INHERIT FALSE, SET TRUE"},
			{"GRANT pg_read_all_data TO " + env.pair.Runtime.User},
			{"GRANT TEMPORARY ON DATABASE " + env.db.Name() + " TO " + env.pair.Runtime.User},
		} {
			exec(t, ctx, env.admin, bypass...)
			if err := postgres.ApplySchema(ctx, env.owner.GormDB, plan); !accessDenied(err) {
				t.Fatalf("schema audit accepted %v: %v", bypass, err)
			}
			// Startup and Health must reject every state that ApplySchema rejects.
			if err := postgres.Readiness(plan)(ctx, env.runtime); !accessDenied(err) {
				t.Fatalf("readiness accepted %v: %v", bypass, err)
			}
			exec(t, ctx, env.admin,
				"REVOKE ALL ON app.secrets FROM "+helper,
				"REVOKE ALL ON FUNCTION app.reveal() FROM "+helper,
				"REVOKE "+helper+" FROM "+env.pair.Runtime.User,
				"REVOKE "+env.pair.Owner.User+" FROM "+env.pair.Runtime.User,
				"REVOKE pg_read_all_data FROM "+env.pair.Runtime.User,
				"REVOKE TEMPORARY ON DATABASE "+env.db.Name()+" FROM "+env.pair.Runtime.User)
		}
		if err := postgres.ApplySchema(ctx, env.owner.GormDB, plan); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rejects unsafe state before mutation", func(t *testing.T) {
		env := newSchemaEnv(t, ctx, fixture)
		plan := schemaPlan(env.pair.Runtime.User)
		exec(t, ctx, env.admin, "GRANT TEMPORARY ON DATABASE "+env.db.Name()+" TO "+env.pair.Runtime.User)
		if err := postgres.ApplySchema(ctx, env.owner.GormDB, plan); !accessDenied(err) || schemaExists(t, ctx, env) {
			t.Fatalf("unsafe runtime role: %v", err)
		}
		exec(t, ctx, env.admin, "REVOKE TEMPORARY ON DATABASE "+env.db.Name()+" FROM "+env.pair.Runtime.User)
		absent := schemaPlan(env.pair.Runtime.User)
		absent.Sets[1].ExpectedVersion = 7
		if err := postgres.ApplySchema(ctx, env.owner.GormDB, absent); apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput || schemaExists(t, ctx, env) {
			t.Fatalf("absent target: %v", err)
		}
		missing := schemaPlan("missing_role")
		if err := postgres.ApplySchema(ctx, env.owner.GormDB, missing); !accessDenied(err) || schemaExists(t, ctx, env) {
			t.Fatalf("missing runtime role: %v", err)
		}
		undeclared := schemaPlan(env.pair.Runtime.User)
		undeclared.RuntimeAccess.Relations = append(undeclared.RuntimeAccess.Relations, postgres.RelationAccess{Name: "ghost", Privileges: []postgres.Privilege{postgres.Select}})
		if err := postgres.ApplySchema(ctx, env.owner.GormDB, undeclared); err == nil {
			t.Fatal("manifest naming a missing relation was accepted")
		}
		if err := postgres.ReadyAccess(ctx, env.runtime.GormDB, undeclared); err == nil {
			t.Fatal("readiness accepted a missing relation")
		}
	})

	t.Run("rejects unsafe role postures and foreign schema ownership", func(t *testing.T) {
		env := newSchemaEnv(t, ctx, fixture)
		runtime := pgx.Identifier{env.pair.Runtime.User}.Sanitize()
		if err := postgres.ReadyAccess(ctx, env.runtime.GormDB, schemaPlan(env.pair.Runtime.User)); !accessDenied(err) ||
			!strings.Contains(apperrors.Normalize(err).Cause.Error(), "schema does not exist") {
			t.Fatalf("readiness before the schema exists: %v", err)
		}
		if err := postgres.ApplySchema(ctx, env.owner.GormDB, schemaPlan(env.pair.Owner.User)); !accessDenied(err) || schemaExists(t, ctx, env) {
			t.Fatalf("migration role accepted as runtime role: %v", err)
		}
		exec(t, ctx, env.admin, "ALTER ROLE "+runtime+" CREATEDB")
		if err := postgres.ApplySchema(ctx, env.owner.GormDB, schemaPlan(env.pair.Runtime.User)); !accessDenied(err) || schemaExists(t, ctx, env) {
			t.Fatalf("privileged runtime attributes accepted: %v", err)
		}
		exec(t, ctx, env.admin, "ALTER ROLE "+runtime+" NOCREATEDB", "CREATE SCHEMA app")
		if err := postgres.ApplySchema(ctx, env.owner.GormDB, schemaPlan(env.pair.Runtime.User)); !accessDenied(err) {
			t.Fatalf("schema owned by another role accepted: %v", err)
		}
		exec(t, ctx, env.admin, "DROP SCHEMA app")
		plan := schemaPlan(env.pair.Runtime.User)
		if err := postgres.ApplySchema(ctx, env.owner.GormDB, plan); err != nil {
			t.Fatal(err)
		}
		exec(t, ctx, env.admin, "ALTER TABLE app.secrets OWNER TO "+runtime)
		if err := postgres.ApplySchema(ctx, env.owner.GormDB, plan); !accessDenied(err) {
			t.Fatalf("runtime ownership of an application object accepted: %v", err)
		}
		exec(t, ctx, env.admin, "ALTER TABLE app.secrets OWNER TO "+pgx.Identifier{env.pair.Owner.User}.Sanitize())
		mislabeled := schemaPlan(env.pair.Runtime.User)
		mislabeled.RuntimeAccess.Sequences = []string{"audit_events_id_seq"}
		mislabeled.RuntimeAccess.Relations = append(mislabeled.RuntimeAccess.Relations,
			postgres.RelationAccess{Name: "widgets_id_seq", Privileges: []postgres.Privilege{postgres.Select}})
		if err := postgres.ApplySchema(ctx, env.owner.GormDB, mislabeled); !accessDenied(err) {
			t.Fatalf("sequence declared as a relation accepted: %v", err)
		}
		if err := postgres.ApplySchema(ctx, env.owner.GormDB, plan); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("resets only the metadata schema under a conflicting search_path", func(t *testing.T) {
		env := newSchemaEnv(t, ctx, fixture)
		exec(t, ctx, asOwner(env), "CREATE SCHEMA other", "CREATE TABLE other.keep(id INTEGER)")
		params := env.pair.Owner
		params.Options = map[string]string{"sslmode": "disable", "search_path": "other"}
		db := open(t, ctx, params)
		cfg := migration.Config{DB: db.GormDB, Driver: postgres.MigrateDriver(), Path: ".", Table: migration.Table{Schema: "app"}, FS: fstest.MapFS{
			"1_app.up.sql": {Data: []byte("CREATE TABLE app.items(id INTEGER)")},
		}}
		exec(t, ctx, asOwner(env), "CREATE SCHEMA app")
		if err := cfg.Up(ctx); err != nil {
			t.Fatal(err)
		}
		if err := cfg.Reset(ctx); err != nil {
			t.Fatal(err)
		}
		var keep bool
		if err := db.WithContext(ctx).Raw("SELECT to_regclass('other.keep') IS NOT NULL").Scan(&keep).Error; err != nil || !keep {
			t.Fatalf("reset dropped a table outside the metadata schema: %v", err)
		}
		if err := cfg.Ready(ctx, 1); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("gates component publication and health on version and access readiness", func(t *testing.T) {
		env := newSchemaEnv(t, ctx, fixture)
		plan := schemaPlan(env.pair.Runtime.User)
		newComponent := func() *database.Component {
			cfg := database.Config{Enabled: true, Params: env.pair.Runtime, MaxRetries: 1, LogLevel: "silent"}
			return database.NewComponent(cfg, logging.NewDefault("test")).WithName("app-db").
				WithDialect(postgres.Dialect()).WithReadiness(postgres.Readiness(plan))
		}
		early := newComponent()
		if err := early.Start(ctx); err == nil || !strings.Contains(err.Error(), "not ready") {
			t.Fatalf("Start before migration = %v", err)
		}
		if _, err := early.DB(); !errors.Is(err, database.ErrNotStarted) {
			t.Fatalf("unready component published: %v", err)
		}
		if err := postgres.ApplySchema(ctx, env.owner.GormDB, plan); err != nil {
			t.Fatal(err)
		}
		comp := newComponent()
		if err := comp.Start(ctx); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := comp.Stop(context.Background()); err != nil {
				t.Error(err)
			}
		})
		if h := comp.Health(ctx); h.Status != "healthy" {
			t.Fatalf("Health = %+v", h)
		}
		exec(t, ctx, asOwner(env), "REVOKE INSERT ON app.audit_events FROM "+pgx.Identifier{env.pair.Runtime.User}.Sanitize())
		if h := comp.Health(ctx); h.Status != "unhealthy" || !strings.Contains(h.Message, "DATABASE_ERROR") {
			t.Fatalf("Health after ACL drift = %+v", h)
		}
		if err := postgres.ApplySchema(ctx, env.owner.GormDB, plan); err != nil {
			t.Fatal(err)
		}
		exec(t, ctx, asOwner(env), "UPDATE app.audit_schema_migrations SET dirty = true")
		if h := comp.Health(ctx); h.Status != "unhealthy" {
			t.Fatalf("Health after version drift = %+v", h)
		}
	})

	t.Run("treats a metadata view as an error", func(t *testing.T) {
		env := newSchemaEnv(t, ctx, fixture)
		exec(t, ctx, asOwner(env), "CREATE SCHEMA app", "CREATE VIEW app.schema_migrations AS SELECT 1::bigint AS version, false AS dirty")
		cfg := migration.Config{DB: env.owner.GormDB, Driver: postgres.MigrateDriver(), Table: migration.Table{Schema: "app"}}
		if err := cfg.Ready(ctx, 1); err == nil || !strings.Contains(err.Error(), "not an ordinary table") {
			t.Fatalf("view metadata: %v", err)
		}
	})
}
