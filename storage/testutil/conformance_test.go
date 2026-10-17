package testutil_test

import (
	"testing"

	"github.com/kbukum/gokit/storage"
	"github.com/kbukum/gokit/storage/local"
	"github.com/kbukum/gokit/storage/testutil"
)

func TestLocalStorageConformance(t *testing.T) {
	store, err := local.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	testutil.Conformance(t, func(*testing.T) storage.Storage { return store })
}

func TestMemoryStorageConformance(t *testing.T) {
	c := testutil.NewComponent()
	if err := c.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	testutil.Conformance(t, func(*testing.T) storage.Storage { return c.Storage() })
}
