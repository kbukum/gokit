package discovery

import (
	"testing"

	"github.com/kbukum/gokit/logging"
)

func TestProviderRegistry_RegisterDuplicateErrors(t *testing.T) {
	r := NewProviderRegistry()
	if err := r.Register("static", func(Config, *logging.Logger) (Registry, Discovery, error) { return nil, nil, nil }); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := r.Register("static", func(Config, *logging.Logger) (Registry, Discovery, error) { return nil, nil, nil }); err == nil {
		t.Fatal("expected error on duplicate provider registration")
	}
}
