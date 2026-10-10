package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"gorm.io/gorm"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/migration"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// Privilege is a table privilege a runtime role may hold on an application relation.
type Privilege string

// Runtime table privileges. TRUNCATE, REFERENCES, TRIGGER and ownership are never granted.
const (
	Select Privilege = "SELECT"
	Insert Privilege = "INSERT"
	Update Privilege = "UPDATE"
	Delete Privilege = "DELETE"
)

var tablePrivileges = []Privilege{Select, Insert, Update, Delete}

// Manifest bounds.
const (
	maxRelations     = 256
	maxSchemaObjects = 4096
	accessTimeout    = 30 * time.Second
)

// RelationAccess grants the runtime role exactly Privileges on one application table or view of the plan schema.
type RelationAccess struct {
	Name       string
	Privileges []Privilege
}

// RuntimeAccess is the runtime role's complete allowlist in one schema. Sequences receive USAGE only, for serial
// inserts. Every version table of the plan receives SELECT only. Anything not listed receives nothing; there are no
// default or future-table grants, so each migration that adds a relation must update the manifest.
type RuntimeAccess struct {
	Role      string
	Relations []RelationAccess
	Sequences []string
}

// SchemaPlan is one application schema, the migration sets whose metadata it holds, and its runtime access.
type SchemaPlan struct {
	Name          string
	Sets          []migration.Set
	RuntimeAccess RuntimeAccess
}

// Validate checks every identifier and target before any database work. Identifiers follow migration.IsIdentifier.
func (p SchemaPlan) Validate() error {
	if !validSchema(p.Name) {
		return apperrors.InvalidInput("schema", "One non-public, non-system application schema is required")
	}
	if len(p.Sets) == 0 {
		return apperrors.InvalidInput("sets", "At least one migration set is required")
	}
	seen := map[string]bool{}
	for _, set := range p.Sets {
		if set.Table.Schema != p.Name {
			return apperrors.InvalidInput("sets", "Migration metadata must live in the plan schema")
		}
		if err := set.Table.Validate(); err != nil {
			return err
		}
		if seen[set.Table.Name] {
			return apperrors.InvalidInput("sets", "Migration metadata tables must be distinct")
		}
		seen[set.Table.Name] = true
	}
	access := p.RuntimeAccess
	if !migration.IsIdentifier(access.Role) || access.Role == "public" || len(access.Role) > 63 {
		return apperrors.InvalidInput("runtime_role", "Runtime role must be a lowercase identifier")
	}
	if len(access.Relations)+len(access.Sequences) > maxRelations {
		return apperrors.InvalidInput("runtime_access", "Runtime access manifest exceeds 256 objects")
	}
	for _, relation := range access.Relations {
		if !migration.IsIdentifier(relation.Name) || seen[relation.Name] || len(relation.Privileges) == 0 {
			return apperrors.InvalidInput("runtime_access", "Each relation needs a distinct identifier and at least one privilege")
		}
		seen[relation.Name] = true
		for i, privilege := range relation.Privileges {
			if !slices.Contains(tablePrivileges, privilege) || slices.Contains(relation.Privileges[:i], privilege) {
				return apperrors.InvalidInput("runtime_access", "Relation privileges must be distinct SELECT, INSERT, UPDATE or DELETE")
			}
		}
	}
	for _, sequence := range access.Sequences {
		if !migration.IsIdentifier(sequence) || seen[sequence] {
			return apperrors.InvalidInput("runtime_access", "Each sequence needs a distinct identifier")
		}
		seen[sequence] = true
	}
	return nil
}

// ApplySchema provisions plan with the migration role on one pinned connection while holding the migration advisory
// lock: it audits the existing runtime role, plans every set, creates the schema owned by the current role when it is
// absent, applies every set to its target, reconciles grants transactionally to the exact manifest, and verifies the
// runtime role's effective privileges before committing the grants. It never creates or edits roles. It works on a pool limited to one connection.
// Rerunning is idempotent; a failure leaves runtime readiness unavailable.
func ApplySchema(ctx context.Context, migrationDB *gorm.DB, plan SchemaPlan) (err error) {
	if util.IsNil(ctx) {
		return apperrors.InvalidInput("context", "Context is required")
	}
	if invalid := plan.Validate(); invalid != nil {
		return invalid
	}
	if migrationDB == nil {
		return apperrors.InvalidInput("database", "Migration database is required")
	}
	pool, err := migrationDB.DB()
	if err != nil {
		return err
	}
	session, err := migration.Begin(ctx, pool, migrationBackend{})
	if err != nil {
		return fmt.Errorf("apply schema %s: %w", plan.Name, err)
	}
	defer func() { err = errors.Join(err, session.Close()) }() //nolint:contextcheck // unlock uses a detached bounded cleanup budget
	conn := session.Conn()
	if err := bounded(ctx, func(ctx context.Context) error { return auditRoles(ctx, conn, plan, true) }); err != nil {
		return err
	}
	if err := session.Plan(ctx, plan.Sets...); err != nil {
		return err
	}
	if err := bounded(ctx, func(ctx context.Context) error { return createSchema(ctx, conn, plan.Name) }); err != nil {
		return err
	}
	if err := session.Apply(ctx, plan.Sets...); err != nil {
		return err
	}
	return bounded(ctx, func(ctx context.Context) error { return reconcileGrants(ctx, conn, plan) })
}

// ReadyAccess verifies, as the runtime role on runtimeDB, that the session user is the plan's runtime role with exactly
// the manifest's privileges: schema USAGE without CREATE, no database CREATE or TEMP, the declared relation and
// sequence rights, and SELECT-only version tables. It runs the same role and undeclared-object audit as ApplySchema,
// so every state ApplySchema rejects also fails readiness. It reads catalogs only; it never locks, migrates or creates
// metadata.
func ReadyAccess(ctx context.Context, runtimeDB *gorm.DB, plan SchemaPlan) error {
	if util.IsNil(ctx) {
		return apperrors.InvalidInput("context", "Context is required")
	}
	if err := plan.Validate(); err != nil {
		return err
	}
	if runtimeDB == nil {
		return apperrors.InvalidInput("database", "Runtime database is required")
	}
	pool, err := runtimeDB.DB()
	if err != nil {
		return err
	}
	var user string
	if err := pool.QueryRowContext(ctx, "SELECT current_user").Scan(&user); err != nil {
		return err
	}
	if user != plan.RuntimeAccess.Role {
		return accessNotReady("session user is not the runtime role")
	}
	if err := auditRoles(ctx, pool, plan, false); err != nil {
		return err
	}
	return auditAccess(ctx, pool, user, plan)
}

// Readiness returns a [database.Check] for [database.Component.WithReadiness] that requires every set of plan at its
// expected clean version, then [ReadyAccess] for the runtime session. It is read-only: no metadata creation, locks or
// migrations, so a component with this check fails Start before migration and reports drift from Health.
func Readiness(plan SchemaPlan) database.Check {
	return func(ctx context.Context, db *database.DB) error {
		if db == nil {
			return apperrors.InvalidInput("database", "Runtime database is required")
		}
		if err := plan.Validate(); err != nil {
			return err
		}
		for _, set := range plan.Sets {
			config := migration.Config{DB: db.GormDB, Driver: MigrateDriver(), Table: set.Table}
			if err := config.Ready(ctx, set.ExpectedVersion); err != nil {
				return fmt.Errorf("migration set %s: %w", set.Table.SQL(), err)
			}
		}
		return ReadyAccess(ctx, db.GormDB, plan)
	}
}

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func bounded(ctx context.Context, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, accessTimeout)
	defer cancel()
	return fn(ctx)
}

func accessNotReady(reason string) error {
	return apperrors.New(apperrors.ErrCodeDatabaseError, "database access is not ready").WithCause(errors.New(reason))
}

func createSchema(ctx context.Context, conn *sql.Conn, schema string) error {
	var owned sql.NullBool
	err := conn.QueryRowContext(ctx,
		"SELECT n.nspowner = (SELECT oid FROM pg_catalog.pg_roles WHERE rolname = current_user) FROM pg_catalog.pg_namespace n WHERE n.nspname = $1",
		schema).Scan(&owned)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = conn.ExecContext(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize())
		return err
	}
	if err != nil {
		return err
	}
	if !owned.Bool {
		return accessNotReady("application schema is not owned by the migration role")
	}
	return nil
}

// auditRoles rejects a runtime role that is missing, is the migration role (when migrating is set, the session user),
// holds role attributes, can create in any schema or the database, owns any object of the plan schema, or is a member
// of any role. The runtime role must be a leaf: memberships, whether inherited, SET-only or neither, would let it reach
// privileges outside the audited matrix.
func auditRoles(ctx context.Context, q querier, plan SchemaPlan, migrating bool) error {
	role := plan.RuntimeAccess.Role
	var exists, migrator, attributes, member, create, owns bool
	err := q.QueryRowContext(ctx, `
		WITH runtime AS (SELECT * FROM pg_catalog.pg_roles WHERE rolname = $1)
		SELECT EXISTS (SELECT 1 FROM runtime),
			$3 AND $1 = current_user,
			EXISTS (SELECT 1 FROM runtime WHERE rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls),
			EXISTS (SELECT 1 FROM pg_catalog.pg_auth_members m, runtime WHERE m.member = runtime.oid),
			EXISTS (SELECT 1 FROM runtime WHERE pg_catalog.has_database_privilege(runtime.oid, current_database(), 'CREATE, TEMPORARY')
				OR EXISTS (SELECT 1 FROM pg_catalog.pg_namespace n WHERE pg_catalog.has_schema_privilege(runtime.oid, n.oid, 'CREATE'))),
			EXISTS (SELECT 1 FROM runtime, pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
				WHERE n.nspname = $2 AND c.relowner = runtime.oid)`,
		role, plan.Name, migrating).Scan(&exists, &migrator, &attributes, &member, &create, &owns)
	if err != nil {
		return err
	}
	switch {
	case !exists:
		return accessNotReady("runtime role does not exist")
	case migrator:
		return accessNotReady("runtime role is the migration role")
	case attributes:
		return accessNotReady("runtime role has privileged attributes")
	case member:
		return accessNotReady("runtime role is a member of another role")
	case create:
		return accessNotReady("runtime role can create objects or temporary tables")
	case owns:
		return accessNotReady("runtime role owns application objects")
	}
	return nil
}

func reconcileGrants(ctx context.Context, conn *sql.Conn, plan SchemaPlan) (err error) {
	schema := pgx.Identifier{plan.Name}.Sanitize()
	grantees := "PUBLIC, " + pgx.Identifier{plan.RuntimeAccess.Role}.Sanitize()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if rbErr := tx.Rollback(); !errors.Is(rbErr, sql.ErrTxDone) {
			err = errors.Join(err, rbErr)
		}
	}()
	var defaults bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_catalog.pg_default_acl d
		WHERE d.defaclrole = (SELECT oid FROM pg_catalog.pg_roles WHERE rolname = current_user)
		AND (d.defaclnamespace = 0 OR d.defaclnamespace = (SELECT oid FROM pg_catalog.pg_namespace WHERE nspname = $1)))`,
		plan.Name).Scan(&defaults); err != nil {
		return err
	}
	if defaults {
		return accessNotReady("migration role has default privileges that grant future objects")
	}
	statements := []string{
		"REVOKE ALL ON SCHEMA " + schema + " FROM " + grantees,
		"REVOKE ALL ON ALL TABLES IN SCHEMA " + schema + " FROM " + grantees,
		"REVOKE ALL ON ALL SEQUENCES IN SCHEMA " + schema + " FROM " + grantees,
		"REVOKE ALL ON ALL ROUTINES IN SCHEMA " + schema + " FROM " + grantees,
		"GRANT USAGE ON SCHEMA " + schema + " TO " + pgx.Identifier{plan.RuntimeAccess.Role}.Sanitize(),
	}
	role := pgx.Identifier{plan.RuntimeAccess.Role}.Sanitize()
	for _, set := range plan.Sets {
		statements = append(statements, "GRANT SELECT ON "+pgx.Identifier{plan.Name, set.Table.Name}.Sanitize()+" TO "+role)
	}
	for _, relation := range plan.RuntimeAccess.Relations {
		privileges := ""
		for i, privilege := range relation.Privileges {
			if i > 0 {
				privileges += ", "
			}
			privileges += string(privilege)
		}
		statements = append(statements, "GRANT "+privileges+" ON "+pgx.Identifier{plan.Name, relation.Name}.Sanitize()+" TO "+role)
	}
	for _, sequence := range plan.RuntimeAccess.Sequences {
		statements = append(statements, "GRANT USAGE ON SEQUENCE "+pgx.Identifier{plan.Name, sequence}.Sanitize()+" TO "+role)
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if err := auditRoles(ctx, tx, plan, true); err != nil {
		return err
	}
	if err := auditAccess(ctx, tx, plan.RuntimeAccess.Role, plan); err != nil {
		return err
	}
	return tx.Commit()
}

// grant is one object's effective privileges for the audited role.
type grant struct {
	kind                                   string
	read, insert, update, remove, sequence bool
	excess                                 bool
}

func expected(plan SchemaPlan) map[string]grant {
	want := map[string]grant{}
	for _, set := range plan.Sets {
		want[set.Table.Name] = grant{read: true}
	}
	for _, relation := range plan.RuntimeAccess.Relations {
		g := grant{}
		for _, privilege := range relation.Privileges {
			switch privilege {
			case Select:
				g.read = true
			case Insert:
				g.insert = true
			case Update:
				g.update = true
			case Delete:
				g.remove = true
			}
		}
		want[relation.Name] = g
	}
	for _, sequence := range plan.RuntimeAccess.Sequences {
		want[sequence] = grant{kind: "S", sequence: true}
	}
	return want
}

// objectPrivileges is the effective-privilege matrix of the plan schema's relations, computed by PostgreSQL itself
// for role, including PUBLIC, inherited and column grants. excess covers prohibited and grant-option privileges,
// including PostgreSQL 17 MAINTAIN, which the CASE keeps unparsed on older servers.
const objectPrivileges = `
SELECT c.relname, c.relkind::text,
	CASE WHEN c.relkind = 'S' THEN false ELSE pg_catalog.has_table_privilege($1, c.oid, 'SELECT') OR pg_catalog.has_any_column_privilege($1, c.oid, 'SELECT') END,
	CASE WHEN c.relkind = 'S' THEN false ELSE pg_catalog.has_table_privilege($1, c.oid, 'INSERT') OR pg_catalog.has_any_column_privilege($1, c.oid, 'INSERT') END,
	CASE WHEN c.relkind = 'S' THEN false ELSE pg_catalog.has_table_privilege($1, c.oid, 'UPDATE') OR pg_catalog.has_any_column_privilege($1, c.oid, 'UPDATE') END,
	CASE WHEN c.relkind = 'S' THEN false ELSE pg_catalog.has_table_privilege($1, c.oid, 'DELETE') END,
	CASE WHEN c.relkind = 'S' THEN pg_catalog.has_sequence_privilege($1, c.oid, 'USAGE') ELSE false END,
	CASE WHEN c.relkind = 'S' THEN pg_catalog.has_sequence_privilege($1, c.oid, 'SELECT, UPDATE, USAGE WITH GRANT OPTION')
		ELSE pg_catalog.has_table_privilege($1, c.oid, 'TRUNCATE, REFERENCES, TRIGGER, SELECT WITH GRANT OPTION, INSERT WITH GRANT OPTION, UPDATE WITH GRANT OPTION, DELETE WITH GRANT OPTION')
			OR pg_catalog.has_any_column_privilege($1, c.oid, 'REFERENCES, SELECT WITH GRANT OPTION, INSERT WITH GRANT OPTION, UPDATE WITH GRANT OPTION')
			OR CASE WHEN pg_catalog.current_setting('server_version_num')::int >= 170000
				THEN pg_catalog.has_table_privilege($1, c.oid, 'MAINTAIN') ELSE false END END
FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = $2 AND c.relkind IN ('r', 'p', 'v', 'm', 'f', 'S')
ORDER BY c.relname LIMIT 4097`

// auditAccess compares role's effective privileges on every relation of the schema with the manifest and rejects any
// callable SECURITY DEFINER routine.
func auditAccess(ctx context.Context, q querier, role string, plan SchemaPlan) (err error) {
	var usage, schemaCreate, databaseCreate, definer bool
	if err = q.QueryRowContext(ctx, `SELECT
		pg_catalog.has_schema_privilege($1, n.oid, 'USAGE'),
		pg_catalog.has_schema_privilege($1, n.oid, 'CREATE, USAGE WITH GRANT OPTION'),
		pg_catalog.has_database_privilege($1, current_database(), 'CREATE, TEMPORARY'),
		EXISTS (SELECT 1 FROM pg_catalog.pg_proc p WHERE p.pronamespace = n.oid AND p.prosecdef
			AND pg_catalog.has_function_privilege($1, p.oid, 'EXECUTE'))
		FROM pg_catalog.pg_namespace n WHERE n.nspname = $2`, role, plan.Name).Scan(&usage, &schemaCreate, &databaseCreate, &definer); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return accessNotReady("application schema does not exist")
		}
		return err
	}
	switch {
	case !usage || schemaCreate:
		return accessNotReady("runtime schema access is not exactly USAGE")
	case databaseCreate:
		return accessNotReady("runtime role can create in the database or use temporary tables")
	case definer:
		return accessNotReady("runtime role can call a SECURITY DEFINER routine")
	}
	want := expected(plan)
	rows, err := q.QueryContext(ctx, objectPrivileges, role, plan.Name)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	found := 0
	for rows.Next() {
		var name string
		var got grant
		if err := rows.Scan(&name, &got.kind, &got.read, &got.insert, &got.update, &got.remove, &got.sequence, &got.excess); err != nil {
			return err
		}
		found++
		if found > maxSchemaObjects {
			return apperrors.InvalidInput("schema", "Application schema exceeds the audited object limit")
		}
		expect, declared := want[name]
		if got.excess {
			return accessNotReady(fmt.Sprintf("runtime role holds prohibited privileges on %s", name))
		}
		if declared && (expect.kind == "S") != (got.kind == "S") {
			return accessNotReady(fmt.Sprintf("%s is not the declared kind of object", name))
		}
		got.kind, expect.kind = "", ""
		if got != expect {
			return accessNotReady(fmt.Sprintf("runtime privileges on %s differ from the manifest", name))
		}
		delete(want, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for name := range want {
		return accessNotReady(fmt.Sprintf("declared object %s does not exist", name))
	}
	return nil
}
