package provider

import (
	"context"
	"errors"
	"sync"
)

// FanOutSink creates a Sink that dispatches each input to multiple sinks in parallel.
// All sinks receive the same input concurrently. Errors from individual sinks
// are joined and returned together.
func FanOutSink[I any](name string, sinks ...Sink[I]) Sink[I] {
	if len(sinks) == 1 {
		return sinks[0]
	}
	return &fanOutSink[I]{name: name, sinks: sinks}
}

type fanOutSink[I any] struct {
	name  string
	sinks []Sink[I]
}

func (f *fanOutSink[I]) Name() string { return f.name }

func (f *fanOutSink[I]) IsAvailable(ctx context.Context) bool {
	for _, s := range f.sinks {
		if s.IsAvailable(ctx) {
			return true
		}
	}
	return false
}

func (f *fanOutSink[I]) Send(ctx context.Context, input I) error {
	var wg sync.WaitGroup
	errs := make([]error, len(f.sinks))

	for i, s := range f.sinks {
		wg.Add(1)
		go func(idx int, sink Sink[I]) {
			defer wg.Done()
			errs[idx] = sink.Send(ctx, input)
		}(i, s)
	}

	wg.Wait()
	return errors.Join(errs...)
}
