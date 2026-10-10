package repository_test

import (
	"testing"

	"github.com/kbukum/gokit/database/query"
	"github.com/kbukum/gokit/database/repository"
	apperrors "github.com/kbukum/gokit/errors"
)

type scopedRow struct {
	ID    int64 `gorm:"primaryKey"`
	Org   string
	Name  string
	Count int
}

type parent struct {
	ID       int64 `gorm:"primaryKey"`
	Org      string
	Children []scopedRow `gorm:"foreignKey:Count"`
}

type permissionRow struct {
	ID        int64  `gorm:"primaryKey"`
	ReadOrg   string `gorm:"->"`
	CreateOrg string `gorm:"<-:create"`
	ReadName  string `gorm:"->"`
	Name      string `gorm:"<-:update"`
	Note      string `gorm:"<-:create"`
}

func validConfig() repository.ScopedConfig[scopedRow, string] {
	return repository.ScopedConfig[scopedRow, string]{
		Resource:       "row",
		ScopeColumns:   []string{"org"},
		BindScope:      func(org string) ([]any, error) { return []any{org}, nil },
		MutableColumns: []string{"name", "count"},
		Query:          query.Config{AllowedFilters: []string{"name"}, AllowedSortFields: []string{"id"}, DefaultSort: "id"},
	}
}

func TestNewScopedSpecValidatesConfigurationOnce(t *testing.T) {
	t.Parallel()
	if _, err := repository.NewScopedSpec[scopedRow, int64, string](validConfig()); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*repository.ScopedConfig[scopedRow, string]){
		"resource":       func(c *repository.ScopedConfig[scopedRow, string]) { c.Resource = "" },
		"binder":         func(c *repository.ScopedConfig[scopedRow, string]) { c.BindScope = nil },
		"no scope":       func(c *repository.ScopedConfig[scopedRow, string]) { c.ScopeColumns = nil },
		"unknown scope":  func(c *repository.ScopedConfig[scopedRow, string]) { c.ScopeColumns = []string{"tenant"} },
		"id as scope":    func(c *repository.ScopedConfig[scopedRow, string]) { c.ScopeColumns = []string{"id"} },
		"mutable scope":  func(c *repository.ScopedConfig[scopedRow, string]) { c.MutableColumns = []string{"org"} },
		"mutable id":     func(c *repository.ScopedConfig[scopedRow, string]) { c.MutableColumns = []string{"id"} },
		"repeated":       func(c *repository.ScopedConfig[scopedRow, string]) { c.MutableColumns = []string{"name", "name"} },
		"non-primary id": func(c *repository.ScopedConfig[scopedRow, string]) { c.IDColumn = "name" },
		"facets":         func(c *repository.ScopedConfig[scopedRow, string]) { c.Query.FacetFields = []string{"name"} },
		"includes": func(c *repository.ScopedConfig[scopedRow, string]) {
			c.Query.IncludeConfig.AllowedPaths = []string{"x"}
		},
		"unknown filter":  func(c *repository.ScopedConfig[scopedRow, string]) { c.Query.AllowedFilters = []string{"secret"} },
		"injected search": func(c *repository.ScopedConfig[scopedRow, string]) { c.Query.SearchFields = []string{"name) OR (1=1"} },
	}
	for name, mutate := range cases {
		cfg := validConfig()
		mutate(&cfg)
		if _, err := repository.NewScopedSpec[scopedRow, int64, string](cfg); apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := repository.NewScopedSpec[scopedRow, string, string](validConfig()); err == nil {
		t.Error("mismatched ID type accepted")
	}
	permissions := func(scope string, mutable ...string) error {
		_, err := repository.NewScopedSpec[permissionRow, int64, string](repository.ScopedConfig[permissionRow, string]{
			Resource: "row", ScopeColumns: []string{scope}, MutableColumns: mutable,
			BindScope: func(org string) ([]any, error) { return []any{org}, nil },
		})
		return err
	}
	if err := permissions("create_org", "name"); err != nil {
		t.Errorf("create-only scope with update-only mutable column rejected: %v", err)
	}
	for name, err := range map[string]error{
		"read-only scope":     permissions("read_org"),
		"read-only mutable":   permissions("create_org", "read_name"),
		"create-only mutable": permissions("create_org", "note"),
	} {
		if err == nil || apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := repository.NewScopedSpec[parent, int64, string](repository.ScopedConfig[parent, string]{
		Resource: "parent", ScopeColumns: []string{"org"}, BindScope: func(org string) ([]any, error) { return []any{org}, nil },
	}); err == nil {
		t.Error("association model accepted")
	}
	if _, err := repository.NewScopedSpec[*scopedRow, int64, string](repository.ScopedConfig[*scopedRow, string]{
		Resource: "pointer", ScopeColumns: []string{"org"}, BindScope: func(org string) ([]any, error) { return []any{org}, nil },
	}); err == nil {
		t.Error("pointer row type accepted")
	}
}

func TestScopedSpecBindRequiresDatabase(t *testing.T) {
	t.Parallel()
	spec, err := repository.NewScopedSpec[scopedRow, int64, string](validConfig())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := spec.Bind(nil); err == nil {
		t.Fatal("nil database bound")
	}
	var missing *repository.ScopedSpec[scopedRow, int64, string]
	if _, err := missing.Bind(nil); apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
		t.Fatal("nil spec bound")
	}
}
