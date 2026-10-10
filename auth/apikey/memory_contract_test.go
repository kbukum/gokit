package apikey_test

import (
	"testing"

	"github.com/kbukum/gokit/auth/apikey"
	"github.com/kbukum/gokit/auth/apikey/apikeytest"
)

func TestMemoryStoreContract(t *testing.T) {
	t.Parallel()
	apikeytest.Run(t, apikeytest.Harness{NewStore: func(t *testing.T) apikey.Store {
		store, err := apikey.NewMemoryStore(64)
		if err != nil {
			t.Fatal(err)
		}
		return store
	}})
}
