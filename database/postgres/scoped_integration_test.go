//go:build integration

package postgres_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"gorm.io/gorm"

	"github.com/kbukum/gokit/database/migration"
	"github.com/kbukum/gokit/database/postgres"
	pgtest "github.com/kbukum/gokit/database/postgres/testutil"
	"github.com/kbukum/gokit/database/repository"
	"github.com/kbukum/gokit/database/repository/repositorytest"
	apperrors "github.com/kbukum/gokit/errors"
)

type task struct {
	ID      int64  `gorm:"primaryKey"`
	Org     string `gorm:"not null"`
	Project string `gorm:"not null"`
	Name    string `gorm:"not null"`
}

type taskScope struct{ Org, Project string }

var partitionSet = fstest.MapFS{
	"1_partitions.up.sql": {Data: []byte(`
		CREATE TABLE projects (org TEXT NOT NULL, id TEXT NOT NULL, PRIMARY KEY (org, id));
		CREATE TABLE tasks (
			id BIGSERIAL PRIMARY KEY,
			org TEXT NOT NULL,
			project TEXT NOT NULL,
			name TEXT NOT NULL,
			CONSTRAINT tasks_project_partition FOREIGN KEY (org, project) REFERENCES projects (org, id)
		);`)},
}

func TestScopedRepositoryOnPostgres(t *testing.T) {
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

	t.Run("shared contract", func(t *testing.T) {
		repositorytest.Run(t, func(t *testing.T) *gorm.DB {
			env := newSchemaEnv(t, ctx, fixture)
			exec(t, ctx, asOwner(env), "CREATE SCHEMA app")
			if err := repositorytest.Migrate(env.owner.GormDB); err != nil {
				t.Fatal(err)
			}
			exec(t, ctx, asOwner(env),
				"GRANT USAGE ON SCHEMA app TO "+env.pair.Runtime.User,
				"GRANT SELECT, INSERT, UPDATE, DELETE ON app.scoped_contract_rows TO "+env.pair.Runtime.User,
				"GRANT USAGE ON SEQUENCE app.scoped_contract_rows_id_seq TO "+env.pair.Runtime.User)
			if err := env.runtime.GormDB.WithContext(ctx).Exec("CREATE TABLE app.denied (id INT)").Error; err == nil {
				t.Fatal("runtime role created a table")
			}
			return env.runtime.GormDB
		})
	})

	t.Run("composite constraints and runtime DDL denial", func(t *testing.T) {
		env := newSchemaEnv(t, ctx, fixture)
		plan := postgres.SchemaPlan{
			Name: "app",
			Sets: []migration.Set{{FS: partitionSet, Path: ".", Table: migration.Table{Schema: "app", Name: "schema_migrations"}, ExpectedVersion: 1}},
			RuntimeAccess: postgres.RuntimeAccess{
				Role: env.pair.Runtime.User,
				Relations: []postgres.RelationAccess{
					{Name: "projects", Privileges: []postgres.Privilege{postgres.Select, postgres.Insert}},
					{Name: "tasks", Privileges: []postgres.Privilege{postgres.Select, postgres.Insert, postgres.Update, postgres.Delete}},
				},
				Sequences: []string{"tasks_id_seq"},
			},
		}
		if err := postgres.ApplySchema(ctx, env.owner.GormDB, plan); err != nil {
			t.Fatal(err)
		}
		runtime := env.runtime.WithContext(ctx)
		if err := runtime.Exec("INSERT INTO projects (org, id) VALUES ('org-a', 'p1'), ('org-b', 'p2')").Error; err != nil {
			t.Fatal(err)
		}
		spec, err := repository.NewScopedSpec[task, int64, taskScope](repository.ScopedConfig[task, taskScope]{
			Resource: "task", ScopeColumns: []string{"org", "project"}, MutableColumns: []string{"name"},
			BindScope: func(s taskScope) ([]any, error) { return []any{s.Org, s.Project}, nil },
		})
		if err != nil {
			t.Fatal(err)
		}
		tasks, err := spec.Bind(env.runtime.GormDB)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tasks.Create(ctx, taskScope{"org-a", "p1"}, task{Name: "ok"}); err != nil {
			t.Fatal(err)
		}
		_, err = tasks.Create(ctx, taskScope{"org-a", "p2"}, task{Name: "cross-partition"})
		if code := apperrors.Normalize(err).Code; err == nil || code == apperrors.ErrCodeNotFound || !strings.Contains(err.Error(), "tasks_project_partition") {
			t.Fatalf("cross-partition parent = %v (%s)", err, code)
		}
		for _, ddl := range []string{"CREATE TABLE app.extra (id INT)", "DROP TABLE app.tasks", "TRUNCATE app.tasks", "ALTER TABLE app.tasks ADD COLUMN x INT", "CREATE TEMP TABLE scratch (id INT)"} {
			if err := runtime.Exec(ddl).Error; err == nil {
				t.Fatalf("runtime role ran %s", ddl)
			}
		}
	})

	t.Run("transaction-local settings", func(t *testing.T) {
		env := newSchemaEnv(t, ctx, fixture)
		db := env.runtime.GormDB.WithContext(ctx)
		if err := postgres.SetLocal(ctx, db, "app.partition", "org-a"); apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
			t.Fatalf("SetLocal outside a transaction = %v", err)
		}
		err := db.Transaction(func(tx *gorm.DB) error {
			for _, name := range []string{"search_path", "app.Partition", "app.partition'--", "a.b.c"} {
				if err := postgres.SetLocal(ctx, tx, name, "x"); apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
					t.Errorf("SetLocal(%q) = %v", name, err)
				}
			}
			if err := postgres.SetLocal(ctx, tx, "app.partition", "org-a'; DROP TABLE x; --"); err != nil {
				return err
			}
			var got string
			if err := tx.Raw("SELECT current_setting('app.partition')").Scan(&got).Error; err != nil {
				return err
			}
			if got != "org-a'; DROP TABLE x; --" {
				t.Errorf("setting inside transaction = %q", got)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		pool, err := env.runtime.GormDB.DB()
		if err != nil {
			t.Fatal(err)
		}
		pool.SetMaxOpenConns(1)
		var after string
		if err := db.Raw("SELECT coalesce(current_setting('app.partition', true), '')").Scan(&after).Error; err != nil || after != "" {
			t.Fatalf("setting leaked past the transaction: %q %v", after, err)
		}
	})
}
