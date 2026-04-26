package messaging

import (
	"context"
	"errors"
	"testing"
)

func TestChainHandlers_Empty(t *testing.T) {
	t.Parallel()

	var called bool
	base := MessageHandler(func(_ context.Context, _ Message) error {
		called = true
		return nil
	})

	chain := ChainHandlers(base)
	if err := chain(context.Background(), Message{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("base handler was not called")
	}
}

func TestChainHandlers_Order(t *testing.T) {
	t.Parallel()

	var order []string

	base := MessageHandler(func(_ context.Context, _ Message) error {
		order = append(order, "base")
		return nil
	})

	mw1 := HandlerMiddleware(func(next MessageHandler) MessageHandler {
		return func(ctx context.Context, msg Message) error {
			order = append(order, "mw1-before")
			err := next(ctx, msg)
			order = append(order, "mw1-after")
			return err
		}
	})

	mw2 := HandlerMiddleware(func(next MessageHandler) MessageHandler {
		return func(ctx context.Context, msg Message) error {
			order = append(order, "mw2-before")
			err := next(ctx, msg)
			order = append(order, "mw2-after")
			return err
		}
	})

	chain := ChainHandlers(base, mw1, mw2)
	if err := chain(context.Background(), Message{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"mw1-before", "mw2-before", "base", "mw2-after", "mw1-after"}
	if len(order) != len(expected) {
		t.Fatalf("order = %v, want %v", order, expected)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %q, want %q", i, order[i], v)
		}
	}
}

func TestChainHandlers_ErrorPropagation(t *testing.T) {
	t.Parallel()

	sentinel := context.DeadlineExceeded
	base := MessageHandler(func(_ context.Context, _ Message) error {
		return sentinel
	})

	var sawError bool
	mw := HandlerMiddleware(func(next MessageHandler) MessageHandler {
		return func(ctx context.Context, msg Message) error {
			err := next(ctx, msg)
			if err != nil {
				sawError = true
			}
			return err
		}
	})

	chain := ChainHandlers(base, mw)
	err := chain(context.Background(), Message{})
	if !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want %v", err, sentinel)
	}
	if !sawError {
		t.Error("middleware did not see the error")
	}
}
