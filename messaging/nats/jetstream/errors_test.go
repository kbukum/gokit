package jetstream

import (
	"errors"
	"fmt"
	"testing"

	natsgo "github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"

	"github.com/kbukum/gokit/messaging"
)

func TestErrorClassifier(t *testing.T) {
	t.Parallel()
	var c ErrorClassifier
	for _, tc := range []struct {
		name                  string
		err                   error
		retryable, connection bool
	}{
		{"nil", nil, false, false},
		{"transient", errTransient, true, false},
		{"closed client", fmt.Errorf("send: %w", messaging.ErrClosed), false, false},
		{"closed connection", natsgo.ErrConnectionClosed, false, true},
		{"reconnecting", natsgo.ErrConnectionReconnecting, true, true},
		{"no servers", natsgo.ErrNoServers, true, true},
		{"payload too large", natsgo.ErrMaxPayload, false, false},
		{"permission", natsgo.ErrPermissionViolation, false, false},
		{"stream not found", natsjs.ErrStreamNotFound, true, false},
		{"rejected", &natsjs.APIError{Code: 400}, false, false},
		{"bad request", &natsjs.APIError{Code: 400, ErrorCode: natsjs.JSErrCodeBadRequest}, false, false},
		{"account resources", &natsjs.APIError{Code: 400, ErrorCode: 10002}, true, false},
		{"no peers", &natsjs.APIError{Code: 400, ErrorCode: 10005}, true, false},
		{"consumer name in use", &natsjs.APIError{Code: 400, ErrorCode: natsjs.JSErrCodeConsumerNameExists}, true, false},
		{"consumer exists", &natsjs.APIError{Code: 400, ErrorCode: natsjs.JSErrCodeConsumerExists}, true, false},
		{"consumer absent", &natsjs.APIError{Code: 400, ErrorCode: natsjs.JSErrCodeConsumerDoesNotExist}, true, false},
		{"max consumers", &natsjs.APIError{Code: 400, ErrorCode: natsjs.JSErrCodeMaximumConsumersLimit}, true, false},
		{"max streams", &natsjs.APIError{Code: 400, ErrorCode: 10027}, true, false},
		{"fetch incomplete", fmt.Errorf("fetch: %w", ErrFetchIncomplete), true, false},
		{"rate limited", &natsjs.APIError{Code: 429}, true, false},
		{"server error", &natsjs.APIError{Code: 503}, true, false},
	} {
		if got := c.IsRetryableError(tc.err); got != tc.retryable {
			t.Errorf("%s: IsRetryableError = %v", tc.name, got)
		}
		if got := c.IsConnectionError(tc.err); got != tc.connection {
			t.Errorf("%s: IsConnectionError = %v", tc.name, got)
		}
	}
	if !errors.Is(fmt.Errorf("x: %w", ErrTerminal), ErrTerminal) {
		t.Fatal("ErrTerminal does not wrap")
	}
}
