//go:build integration

package testutil_test

import (
	"testing"

	"github.com/kbukum/gokit/database/postgres/testutil"
)

func TestFixtureContainerIdentityEndsAtClose(t *testing.T) {
	t.Parallel()
	f, err := testutil.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.Close(t.Context()); err != nil {
			t.Error(err)
		}
	})
	if f.ContainerID() == "" {
		t.Fatal("running fixture omitted its container identity")
	}
	if err := f.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if f.ContainerID() != "" {
		t.Fatal("released fixture retained its container identity")
	}
}
