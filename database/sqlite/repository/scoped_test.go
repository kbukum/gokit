package repository_test

import (
	"testing"

	"gorm.io/gorm"

	"github.com/kbukum/gokit/database/repository/repositorytest"
)

func TestScopedRepositoryContract(t *testing.T) {
	repositorytest.Run(t, func(t *testing.T) *gorm.DB {
		db := setupTestDB(t)
		if err := repositorytest.Migrate(db); err != nil {
			t.Fatal(err)
		}
		return db
	})
}
