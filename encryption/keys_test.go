package encryption_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/encryption"
)

func TestFileKeysValidateReferencesAndCancellation(t *testing.T) {
	t.Parallel()
	for _, files := range [][]encryption.KeyFile{
		nil,
		{{ID: "", Path: "/run/secrets/master"}},
		{{ID: "current", Path: ""}},
		{{ID: " current", Path: "/run/secrets/master"}},
		{{ID: "current", Path: "/one"}, {ID: "current", Path: "/two"}},
	} {
		if source, err := encryption.NewFileKeys(files); source != nil || err == nil {
			t.Fatal("invalid or ambiguous key references accepted")
		}
	}
	source, err := encryption.NewFileKeys([]encryption.KeyFile{{ID: "current", Path: "/run/secrets/master"}})
	if err != nil {
		t.Fatal(err)
	}
	if key, err := source.Key(t.Context(), "unknown"); !key.IsEmpty() || err == nil {
		t.Fatal("unknown key succeeded")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if key, err := source.Key(ctx, "current"); !key.IsEmpty() || !errors.Is(err, context.Canceled) {
		t.Fatal("canceled key access did not preserve cancellation")
	}
	if key, err := source.Key(nil, "current"); !key.IsEmpty() || err == nil { //nolint:staticcheck // Exercise the nil-context boundary.
		t.Fatal("nil key context accepted")
	}
}
