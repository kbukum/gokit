package worker

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/provider"
)

// FromProvider wraps a provider.RequestResponse as a Handler.
// The handler checks IsAvailable before executing; returns an error if unavailable.
// Emits a single EventResult on success. No progress events.
func FromProvider[I, O any](p provider.RequestResponse[I, O]) Handler[I, O] {
	return HandlerFunc[I, O](func(ctx context.Context, task I, emit func(Event[O])) error {
		if !p.IsAvailable(ctx) {
			return fmt.Errorf("worker: provider %q is not available", p.Name())
		}
		result, err := p.Execute(ctx, task)
		if err != nil {
			return err
		}
		emit(resultEvent(result))
		return nil
	})
}

// AsProviderConfig configures how a Handler maps to a provider.
type AsProviderConfig struct {
	// ProviderName identifies this provider (implements provider.Provider.Name).
	ProviderName string `yaml:"provider_name" mapstructure:"provider_name"`
}

// AsProvider wraps a Handler as a provider.RequestResponse.
// Runs the handler, waits for completion, returns the final EventResult data.
// Progress and partial events are discarded.
func AsProvider[I, O any](h Handler[I, O], cfg AsProviderConfig) provider.RequestResponse[I, O] {
	return &handlerProvider[I, O]{handler: h, name: cfg.ProviderName}
}

// compile-time assertion
var _ provider.RequestResponse[any, any] = (*handlerProvider[any, any])(nil)

type handlerProvider[I, O any] struct {
	handler Handler[I, O]
	name    string
}

func (hp *handlerProvider[I, O]) Name() string                       { return hp.name }
func (hp *handlerProvider[I, O]) IsAvailable(_ context.Context) bool { return true }

func (hp *handlerProvider[I, O]) Execute(ctx context.Context, input I) (O, error) {
	var result O
	var resultSet bool

	emit := func(e Event[O]) {
		if e.Type == EventResult {
			result = e.Data
			resultSet = true
		}
	}

	err := hp.handler.Handle(ctx, input, emit)
	if err != nil {
		var zero O
		return zero, err
	}

	if !resultSet {
		// Handler succeeded without emitting EventResult — return zero value
		return result, nil
	}
	return result, nil
}
