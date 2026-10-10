package session

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
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

func TestAcquireLookupOverBudgetIsTimeoutNotClosed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &memoryStore{rows: make(map[string]Record)}
		manager, _ := fixture(t, store)
		defer manager.Close(context.Background())
		issued, err := manager.Create(context.Background(), caller())
		if err != nil {
			t.Fatal(err)
		}
		store.mu.Lock()
		store.lookupLag = 2 * LookupBudget
		store.mu.Unlock()
		_, _, err = manager.Acquire(context.Background(), issued.Principal.Reference)
		if got := apperrors.Normalize(err); got.Code != apperrors.ErrCodeTimeout {
			t.Fatalf("over-budget admission classified as %v (%v)", got.Code, err)
		}
	})
}
