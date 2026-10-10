package apikeytest_test

import (
	"testing"

	"github.com/kbukum/gokit/auth/apikey"
	"github.com/kbukum/gokit/auth/apikey/apikeytest"
)

// TestRunPassesReferenceStore proves the contract itself is satisfiable: the reference memory store passes it with
// the default owner and with an explicit one.
func TestRunPassesReferenceStore(t *testing.T) {
	t.Parallel()
	newStore := func(t *testing.T) apikey.Store {
		store, err := apikey.NewMemoryStore(64)
		if err != nil {
			t.Fatal(err)
		}
		return store
	}
	t.Run("DefaultOwner", func(t *testing.T) {
		t.Parallel()
		apikeytest.Run(t, apikeytest.Harness{NewStore: newStore})
	})
	t.Run("ExplicitOwner", func(t *testing.T) {
		t.Parallel()
		apikeytest.Run(t, apikeytest.Harness{NewStore: newStore, OwnerID: "user-7"})
	})
}
