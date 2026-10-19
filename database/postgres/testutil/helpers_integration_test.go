//go:build integration

package testutil_test

import (
	"testing"

	"gorm.io/gorm"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/postgres"
)

// preparedDialector opens a pool through the adapter's public Prepare path and closes it at cleanup.
func preparedDialector(t *testing.T, params database.ConnParams) gorm.Dialector {
	t.Helper()
	opener, err := postgres.Prepare(t.Context(), database.ConnectionInput{Params: params})
	if err != nil {
		t.Fatal(err)
	}
	dialector, err := opener.Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	owner, ok := dialector.(interface{ Close() error })
	if !ok {
		t.Fatal("PostgreSQL dialector has no pool owner")
	}
	t.Cleanup(func() {
		if err := owner.Close(); err != nil {
			t.Error(err)
		}
	})
	return dialector
}
