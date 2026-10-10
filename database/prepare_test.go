package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kbukum/gokit/util"
)

type resolvingDialect struct{}

func (resolvingDialect) Name() string { return "resolving" }
func (resolvingDialect) Prepare(ctx context.Context, input ConnectionInput) (Opener, error) {
	_, err := ResolvePassword(ctx, input.Params, input.Secrets)
	return nil, err
}

type deadlineSecret struct{ t *testing.T }

func (s deadlineSecret) ReadSecret(ctx context.Context, _ string) (util.SecretString, error) {
	deadline, exists := ctx.Deadline()
	if !exists || time.Until(deadline) > 50*time.Millisecond {
		s.t.Error("preparation did not inherit the configured operation deadline")
		return util.SecretString{}, errors.New("missing preparation deadline")
	}
	<-ctx.Done()
	return util.SecretString{}, ctx.Err()
}

func TestInjectedSecretPreparationHasConfiguredDeadline(t *testing.T) {
	t.Parallel()
	db, err := NewWithContext(t.Context(), resolvingDialect{},
		Config{Params: ConnParams{PasswordFile: "injected"}, ConnectTimeout: "50ms"},
		testLogger(), WithSecretSource(deadlineSecret{t: t}))
	if db != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("credential preparation lost its deadline: %v", err)
	}
}
