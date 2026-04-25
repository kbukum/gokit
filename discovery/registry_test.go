package discovery

import (
	"testing"

	"github.com/kbukum/gokit/logger"
)

func TestProviderRegistry_RegisterDuplicatePanics(t *testing.T) {
	r := NewProviderRegistry()
	r.Register("static", func(Config, *logger.Logger) (Registry, Discovery, error) { return nil, nil, nil })

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate provider registration")
		}
	}()
	r.Register("static", func(Config, *logger.Logger) (Registry, Discovery, error) { return nil, nil, nil })
}
