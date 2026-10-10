// Package repositorytest provides shared contracts for partition-enforced repositories, so every database adapter
// proves the same scoped behavior.
package repositorytest

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"github.com/kbukum/gokit/database/query"
	"github.com/kbukum/gokit/database/repository"
	apperrors "github.com/kbukum/gokit/errors"
)

// Partition is the contract's two-column scope.
type Partition struct{ Org, Project string }

// Row is the contract's scalar model. Its hooks fail, proving scoped repositories never run them.
type Row struct {
	ID      uint   `gorm:"primaryKey"`
	Org     string `gorm:"not null"`
	Project string `gorm:"not null"`
	Name    string `gorm:"not null"`
	Count   int    `gorm:"not null"`
	Payload []byte
}

// TableName keeps the contract table distinct from adapter fixtures.
func (Row) TableName() string { return "scoped_contract_rows" }

var errHook = errors.New("scoped repository ran a model hook")

// BeforeCreate fails if hooks run.
func (Row) BeforeCreate(*gorm.DB) error { return errHook }

// BeforeUpdate fails if hooks run.
func (Row) BeforeUpdate(*gorm.DB) error { return errHook }

// BeforeDelete fails if hooks run.
func (Row) BeforeDelete(*gorm.DB) error { return errHook }

// AfterFind fails if hooks run.
func (Row) AfterFind(*gorm.DB) error { return errHook }

// Spec returns the contract specification.
func Spec(t testing.TB) *repository.ScopedSpec[Row, uint, Partition] {
	t.Helper()
	spec, err := repository.NewScopedSpec[Row, uint, Partition](repository.ScopedConfig[Row, Partition]{
		Resource:     "contract row",
		ScopeColumns: []string{"org", "project"},
		BindScope: func(p Partition) ([]any, error) {
			return []any{p.Org, p.Project}, nil
		},
		MutableColumns: []string{"name", "count", "payload"},
		Query: query.Config{
			SearchFields: []string{"name"}, AllowedFilters: []string{"name", "count"}, AllowedSortFields: []string{"id", "name"},
			DefaultSort: "id", MaxPageSize: 2,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

// Migrate creates the contract table on db without running model hooks.
func Migrate(db *gorm.DB) error {
	return db.Session(&gorm.Session{SkipHooks: true}).AutoMigrate(&Row{})
}

// Run proves the scoped repository contract. open returns a fresh database whose contract table exists (see
// [Migrate]); each subtest uses its own database.
func Run(t *testing.T, open func(t *testing.T) *gorm.DB) {
	t.Helper()
	spec := Spec(t)
	alpha, beta := Partition{"org-a", "project-a"}, Partition{"org-b", "project-a"}
	setup := func(t *testing.T) (*gorm.DB, *repository.ScopedRepository[Row, uint, Partition]) {
		t.Helper()
		db := open(t)
		repo, err := spec.Bind(db)
		if err != nil {
			t.Fatal(err)
		}
		return db, repo
	}
	create := func(t *testing.T, repo *repository.ScopedRepository[Row, uint, Partition], scope Partition, row Row) Row {
		t.Helper()
		created, err := repo.Create(t.Context(), scope, row)
		if err != nil {
			t.Fatal(err)
		}
		return created
	}

	t.Run("create stamps scope without mutating input", func(t *testing.T) {
		_, repo := setup(t)
		input := Row{Name: "first", Payload: []byte("bytes")}
		created := create(t, repo, alpha, input)
		if created.ID == 0 || created.Org != alpha.Org || created.Project != alpha.Project || input.Org != "" || input.ID != 0 {
			t.Fatalf("created %+v from %+v", created, input)
		}
		if _, err := repo.Create(t.Context(), alpha, Row{Org: beta.Org, Name: "smuggled"}); code(err) != apperrors.ErrCodeInvalidInput {
			t.Fatalf("conflicting inserted scope = %v", err)
		}
		matching := create(t, repo, alpha, Row{Org: alpha.Org, Project: alpha.Project, Name: "explicit"})
		if matching.Org != alpha.Org {
			t.Fatalf("matching explicit scope = %+v", matching)
		}
	})

	t.Run("missing and out-of-scope rows are indistinguishable", func(t *testing.T) {
		_, repo := setup(t)
		row := create(t, repo, alpha, Row{Name: "private"})
		got, err := repo.Get(t.Context(), alpha, row.ID)
		if err != nil || got.Name != "private" {
			t.Fatalf("Get in scope = %+v %v", got, err)
		}
		_, foreign := repo.Get(t.Context(), beta, row.ID)
		_, missing := repo.Get(t.Context(), alpha, row.ID+100)
		if code(foreign) != apperrors.ErrCodeNotFound || missing == nil || foreign.Error() != missing.Error() {
			t.Fatalf("foreign %v, missing %v", foreign, missing)
		}
	})

	t.Run("lists are bounded and partitioned through OR search", func(t *testing.T) {
		_, repo := setup(t)
		for _, name := range []string{"apple", "apricot", "avocado"} {
			create(t, repo, alpha, Row{Name: name})
		}
		create(t, repo, beta, Row{Name: "apple"})
		result, err := repo.List(t.Context(), alpha, query.Params{PageSize: 50, Query: query.FilterQuery{FreeText: "ap"}})
		if err != nil {
			t.Fatal(err)
		}
		if result.Pagination.Total != 2 || len(result.Data) != 2 || result.Pagination.PageSize != 2 {
			t.Fatalf("search page = %+v", result.Pagination)
		}
		for _, row := range result.Data {
			if row.Org != alpha.Org {
				t.Fatalf("listed foreign row %+v", row)
			}
		}
		all, err := repo.List(t.Context(), alpha, query.Params{PageSize: 50})
		if err != nil || all.Pagination.Total != 3 || len(all.Data) != 2 {
			t.Fatalf("bounded page = %+v %v", all, err)
		}
		bad := query.Params{}
		bad.AddCondition("org", query.OpEq, beta.Org)
		if _, err := repo.List(t.Context(), alpha, bad); code(err) != apperrors.ErrCodeInvalidInput {
			t.Fatalf("unlisted filter = %v", err)
		}
		includes := query.Params{Includes: query.IncludeSet{Paths: []query.IncludePath{{Parts: []string{"owner"}, Raw: "owner"}}}}
		if _, err := repo.List(t.Context(), alpha, includes); code(err) != apperrors.ErrCodeInvalidInput {
			t.Fatalf("includes = %v", err)
		}
	})

	t.Run("update writes mutable columns including zeros and never inserts", func(t *testing.T) {
		db, repo := setup(t)
		row := create(t, repo, alpha, Row{Name: "before", Count: 7, Payload: []byte("x")})
		if err := repo.Update(t.Context(), alpha, row.ID, Row{Name: "after"}); err != nil {
			t.Fatal(err)
		}
		got, err := repo.Get(t.Context(), alpha, row.ID)
		if err != nil || got.Name != "after" || got.Count != 0 || len(got.Payload) != 0 {
			t.Fatalf("updated row = %+v %v", got, err)
		}
		if err := repo.Update(t.Context(), alpha, row.ID, Row{ID: row.ID + 1, Name: "moved"}); code(err) != apperrors.ErrCodeInvalidInput {
			t.Fatalf("identity update = %v", err)
		}
		if err := repo.Update(t.Context(), alpha, row.ID, Row{Org: beta.Org, Name: "moved"}); code(err) != apperrors.ErrCodeInvalidInput {
			t.Fatalf("partition update = %v", err)
		}
		if err := repo.Update(t.Context(), beta, row.ID, Row{Name: "foreign"}); code(err) != apperrors.ErrCodeNotFound {
			t.Fatalf("foreign update = %v", err)
		}
		if err := repo.Update(t.Context(), alpha, row.ID+100, Row{Name: "upsert"}); code(err) != apperrors.ErrCodeNotFound {
			t.Fatalf("missing update = %v", err)
		}
		var count int64
		if err := db.Session(&gorm.Session{SkipHooks: true}).Model(&Row{}).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("rows after updates = %d %v", count, err)
		}
	})

	t.Run("delete is scoped", func(t *testing.T) {
		_, repo := setup(t)
		row := create(t, repo, alpha, Row{Name: "doomed"})
		if err := repo.Delete(t.Context(), beta, row.ID); code(err) != apperrors.ErrCodeNotFound {
			t.Fatalf("foreign delete = %v", err)
		}
		if err := repo.Delete(t.Context(), alpha, row.ID); err != nil {
			t.Fatal(err)
		}
		if err := repo.Delete(t.Context(), alpha, row.ID); code(err) != apperrors.ErrCodeNotFound {
			t.Fatalf("repeated delete = %v", err)
		}
	})

	t.Run("invalid scopes are rejected before SQL", func(t *testing.T) {
		_, repo := setup(t)
		for _, scope := range []Partition{{}, {Org: "org-a"}, {Project: "project-a"}} {
			if _, err := repo.Get(t.Context(), scope, 1); code(err) != apperrors.ErrCodeInvalidInput {
				t.Fatalf("scope %+v = %v", scope, err)
			}
			if _, err := repo.Create(t.Context(), scope, Row{Name: "x"}); code(err) != apperrors.ErrCodeInvalidInput {
				t.Fatalf("create scope %+v = %v", scope, err)
			}
		}
	})

	t.Run("binding discards polluted statement clauses", func(t *testing.T) {
		db, _ := setup(t)
		repo, err := spec.Bind(db.Where("1 = 0").Limit(0))
		if err != nil {
			t.Fatal(err)
		}
		row := create(t, repo, alpha, Row{Name: "visible"})
		if _, err := repo.Get(t.Context(), alpha, row.ID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("transaction binding reads its writes and rolls back", func(t *testing.T) {
		db, repo := setup(t)
		var id uint
		rollback := errors.New("rollback")
		err := db.Transaction(func(tx *gorm.DB) error {
			scoped, err := spec.Bind(tx)
			if err != nil {
				return err
			}
			row, err := scoped.Create(t.Context(), alpha, Row{Name: "pending"})
			if err != nil {
				return err
			}
			id = row.ID
			if _, err := scoped.Get(t.Context(), alpha, id); err != nil {
				return err
			}
			return rollback
		})
		if !errors.Is(err, rollback) {
			t.Fatalf("transaction = %v", err)
		}
		if _, err := repo.Get(t.Context(), alpha, id); code(err) != apperrors.ErrCodeNotFound {
			t.Fatalf("rolled-back row = %v", err)
		}
	})

	t.Run("canceled context is preserved", func(t *testing.T) {
		_, repo := setup(t)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := repo.Get(ctx, alpha, 1); !errors.Is(err, context.Canceled) && code(err) != apperrors.ErrCodeCanceled {
			t.Fatalf("canceled Get = %v", err)
		}
	})
}

func code(err error) apperrors.ErrorCode {
	if err == nil {
		return ""
	}
	return apperrors.Normalize(err).Code
}
