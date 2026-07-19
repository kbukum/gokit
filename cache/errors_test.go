package cache

import (
	"strings"
	"testing"
)

func TestConfigTypeErrorMessage(t *testing.T) {
	t.Parallel()

	err := (&ConfigTypeError{Provider: ProviderMemory, Expected: "*cache.MemoryConfig", Actual: 1}).Error()
	for _, want := range []string{ProviderMemory, "*cache.MemoryConfig", "int"} {
		if !strings.Contains(err, want) {
			t.Fatalf("ConfigTypeError = %q, missing %q", err, want)
		}
	}
}
