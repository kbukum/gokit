package postgres_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/migration"
	"github.com/kbukum/gokit/database/postgres"
	"github.com/kbukum/gokit/logging"
)

func TestMigrateRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	log := logging.NewDefault("test")
	plan := postgres.SchemaPlan{
		Name:          "app",
		Sets:          []migration.Set{{FS: fstest.MapFS{"1_a.up.sql": {Data: []byte("SELECT 1")}}, Path: ".", Table: migration.Table{Schema: "app", Name: "widget_schema_migrations"}, ExpectedVersion: 1}},
		RuntimeAccess: postgres.RuntimeAccess{Role: "app_runtime"},
	}
	valid := postgres.SchemaMigration{Config: database.Config{Params: database.ConnParams{Host: "127.0.0.1", Port: 1, Database: "x", User: "u"}}, Plan: plan}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for name, c := range map[string]struct {
		ctx context.Context
		log *logging.Logger
		m   postgres.SchemaMigration
	}{
		"no logger":       {context.Background(), nil, valid},
		"invalid plan":    {context.Background(), log, postgres.SchemaMigration{Config: valid.Config}},
		"negative budget": {context.Background(), log, postgres.SchemaMigration{Config: valid.Config, Plan: plan, SetBudget: -1}},
		"ended context":   {canceled, log, valid},
	} {
		if err := postgres.Migrate(c.ctx, c.log, c.m); err == nil {
			t.Fatalf("%s: want error", name)
		}
	}
}
