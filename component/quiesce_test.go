package component_test

import (
	"errors"
	"testing"

	"github.com/kbukum/gokit/component"
	componenttest "github.com/kbukum/gokit/component/testutil"
)

func TestShutdownQuiescesOnceAndPreservesErrors(t *testing.T) {
	t.Parallel()
	registry := component.NewRegistry()
	failure := errors.New("quiesce failed")
	calls := make(map[string]int)
	for _, name := range []string{"first", "second", "third"} {
		c := &componenttest.Component{ComponentName: name, QuiesceFunc: func() error {
			calls[name]++
			if name == "first" && calls[name] == 1 {
				return failure
			}
			return nil
		}}
		if err := registry.Register(c); err != nil {
			t.Fatal(err)
		}
	}
	if err := registry.StartAll(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := registry.Shutdown(t.Context(), nil); !errors.Is(err, failure) {
		t.Errorf("lost quiesce error: %v", err)
	}
	for name, count := range calls {
		if count != 1 {
			t.Errorf("%s quiesced %d times, want 1", name, count)
		}
	}
}
