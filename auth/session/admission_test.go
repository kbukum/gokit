package session

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestAdmissionCancellationAndBudgetsBeforeGate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		manager, _ := fixture(t, &memoryStore{rows: make(map[string]Record)})
		defer manager.Close(context.Background())
		issued, err := manager.Create(context.Background(), caller())
		if err != nil {
			t.Fatal(err)
		}
		entered, releaseGate, gateDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
		go func() {
			err := manager.gate.Execute(context.Background(), func() error { close(entered); <-releaseGate; return nil })
			if err != nil {
				t.Error(err)
			}
			close(gateDone)
		}()
		<-entered
		defer func() { close(releaseGate); <-gateDone }()
		for _, operation := range []struct {
			name   string
			budget time.Duration
			call   func(context.Context) error
		}{
			{"create", MutationBudget, func(ctx context.Context) error { _, err := manager.Create(ctx, caller()); return err }},
			{"rotate", MutationBudget, func(ctx context.Context) error {
				_, err := manager.Rotate(ctx, issued.Principal.Reference, caller())
				return err
			}},
			{"logout", MutationBudget, func(ctx context.Context) error { return manager.Logout(ctx, issued.Principal.Reference) }},
			{"acquire", LookupBudget, func(ctx context.Context) error {
				_, _, err := manager.Acquire(ctx, issued.Principal.Reference)
				return err
			}},
		} {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := operation.call(ctx); !errors.Is(err, context.Canceled) {
				t.Fatal(operation.name, "ignored cancellation", err)
			}
			start := time.Now()
			if err := operation.call(context.Background()); err == nil {
				t.Fatal(operation.name, "bypassed held gate")
			}
			if elapsed := time.Since(start); elapsed > operation.budget {
				t.Fatal(operation.name, "budget started after gate", elapsed)
			}
		}
	})
}
