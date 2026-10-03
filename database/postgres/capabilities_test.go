package postgres_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/kbukum/gokit/database/postgres"
)

func TestPostgresOptionalCapabilities(t *testing.T) {
	t.Parallel()
	dialect := postgres.Open("host=localhost dbname=app")
	if _, ok := dialect.(gorm.SavePointerDialectorInterface); !ok {
		t.Error("savepoint support missing")
	}
	translator, ok := dialect.(gorm.ErrorTranslator)
	if !ok {
		t.Fatal("error translation missing")
	}
	if err := translator.Translate(&pgconn.PgError{Code: "23505"}); !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("duplicate key not translated: %v", err)
	}
	if err := translator.Translate(&pgconn.PgError{Code: "23503"}); !errors.Is(err, gorm.ErrForeignKeyViolated) {
		t.Fatalf("foreign key not translated: %v", err)
	}
	configurer, ok := dialect.(interface{ Apply(*gorm.Config) error })
	if !ok {
		t.Fatal("configuration hook missing")
	}
	config := &gorm.Config{}
	if err := configurer.Apply(config); err != nil {
		t.Fatal(err)
	}
	naming, ok := config.NamingStrategy.(schema.NamingStrategy)
	if !ok || naming.IdentifierMaxLength != 63 {
		t.Fatalf("PostgreSQL identifier limit lost: %+v", config.NamingStrategy)
	}
}
