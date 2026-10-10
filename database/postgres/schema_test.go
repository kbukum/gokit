package postgres

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/migration"
	apperrors "github.com/kbukum/gokit/errors"
)

func validPlan() SchemaPlan {
	return SchemaPlan{
		Name: "app",
		Sets: []migration.Set{{Table: migration.Table{Schema: "app", Name: "schema_migrations"}, ExpectedVersion: 1}},
		RuntimeAccess: RuntimeAccess{
			Role:      "runtime",
			Relations: []RelationAccess{{Name: "items", Privileges: []Privilege{Select, Insert}}},
			Sequences: []string{"items_id_seq"},
		},
	}
}

func isInvalidInput(err error) bool {
	return err != nil && apperrors.Normalize(err).Code == apperrors.ErrCodeInvalidInput
}

func TestSchemaPlanValidateRejectsEachUnsafeManifest(t *testing.T) {
	t.Parallel()
	if err := validPlan().Validate(); err != nil {
		t.Fatal(err)
	}
	many := make([]string, maxRelations)
	for i := range many {
		many[i] = "s" + string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	for name, mutate := range map[string]func(*SchemaPlan){
		"public schema":        func(p *SchemaPlan) { p.Name = "public" },
		"no sets":              func(p *SchemaPlan) { p.Sets = nil },
		"metadata elsewhere":   func(p *SchemaPlan) { p.Sets[0].Table.Schema = "other" },
		"invalid table":        func(p *SchemaPlan) { p.Sets[0].Table.Name = "Bad" },
		"duplicate metadata":   func(p *SchemaPlan) { p.Sets = append(p.Sets, p.Sets[0]) },
		"invalid role":         func(p *SchemaPlan) { p.RuntimeAccess.Role = "Runtime" },
		"public role":          func(p *SchemaPlan) { p.RuntimeAccess.Role = "public" },
		"oversized manifest":   func(p *SchemaPlan) { p.RuntimeAccess.Sequences = many },
		"relation is metadata": func(p *SchemaPlan) { p.RuntimeAccess.Relations[0].Name = "schema_migrations" },
		"no privileges":        func(p *SchemaPlan) { p.RuntimeAccess.Relations[0].Privileges = nil },
		"truncate privilege":   func(p *SchemaPlan) { p.RuntimeAccess.Relations[0].Privileges = []Privilege{"TRUNCATE"} },
		"repeated privilege":   func(p *SchemaPlan) { p.RuntimeAccess.Relations[0].Privileges = []Privilege{Select, Select} },
		"sequence is relation": func(p *SchemaPlan) { p.RuntimeAccess.Sequences = []string{"items"} },
		"invalid sequence":     func(p *SchemaPlan) { p.RuntimeAccess.Sequences = []string{"Seq"} },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			plan := validPlan()
			mutate(&plan)
			if err := plan.Validate(); !isInvalidInput(err) {
				t.Fatalf("Validate = %v", err)
			}
		})
	}
}

func TestSchemaEntryPointsRejectInvalidArgumentsBeforeDatabaseWork(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	var nilCtx context.Context
	invalid := validPlan()
	invalid.Name = "public"
	for name, err := range map[string]error{
		"apply nil context":     ApplySchema(nilCtx, nil, validPlan()),
		"apply invalid plan":    ApplySchema(ctx, nil, invalid),
		"apply nil database":    ApplySchema(ctx, nil, validPlan()),
		"ready nil context":     ReadyAccess(nilCtx, nil, validPlan()),
		"ready invalid plan":    ReadyAccess(ctx, nil, invalid),
		"ready nil database":    ReadyAccess(ctx, nil, validPlan()),
		"readiness nil db":      Readiness(validPlan())(ctx, nil),
		"readiness invalid":     Readiness(invalid)(ctx, &database.DB{}),
		"metadata public table": MigrationBackend().ValidateTable(migration.Table{Schema: "public", Name: "schema_migrations"}),
	} {
		if !isInvalidInput(err) {
			t.Errorf("%s = %v", name, err)
		}
	}
	if err := MigrationBackend().ValidateTable(migration.Table{Schema: "app", Name: "schema_migrations"}); err != nil {
		t.Fatal(err)
	}
}
