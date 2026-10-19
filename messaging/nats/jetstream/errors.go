package jetstream

import (
	"errors"
	"net/http"
	"slices"

	natsgo "github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"

	"github.com/kbukum/gokit/messaging"
)

// ErrDeliveryLimit is reported to OnError when a message fails its last attempt and [DeliveryPolicy].DiscardExhausted gives it up. The message stays in the stream for replay.
var ErrDeliveryLimit = errors.New("jetstream: delivery limit reached")

// ErrUnsafeBoundConsumer ends Consume when ConsumerConfig.Bind finds a durable consumer that could lose a failed message. Its ack policy must be explicit, because acknowledging a later message under ack-all also settles an earlier failed one. Its max_deliver must be -1, because a server-side limit stops redelivery before a failed hand-off is retried; [DeliveryPolicy].MaxAttempts bounds attempts instead.
var ErrUnsafeBoundConsumer = errors.New("jetstream: bound consumer can lose messages")

// ErrNotReceiving is the [Consumer.ConsumeStatus] of a consumer that has not completed a fetch yet.
var ErrNotReceiving = errors.New("jetstream: not receiving yet")

// ErrFetchIncomplete marks a fetch that reached its deadline before the server completed it. An idle server ends a fetch on time, so this usually means the pull request never reached it, for example because a permission dropped it, but a slow link can cause it too. It is retryable.
var ErrFetchIncomplete = errors.New("jetstream: fetch did not complete in time")

var _ messaging.ErrorClassifier = ErrorClassifier{}

// ErrorClassifier classifies JetStream failures for retry decisions.
type ErrorClassifier struct{}

// IsConnectionError reports a lost or unreachable connection.
func (ErrorClassifier) IsConnectionError(err error) bool {
	return errors.Is(err, natsgo.ErrConnectionClosed) || errors.Is(err, natsgo.ErrConnectionReconnecting) ||
		errors.Is(err, natsgo.ErrNoServers) || messaging.IsConnectionError(err)
}

// IsRetryableError reports a failure that a later attempt may not repeat.
func (ErrorClassifier) IsRetryableError(err error) bool { return err != nil && !permanent(err) }

// permanent reports a failure that retrying cannot fix: a closed client or connection, a message the server never accepts, a missing permission, or a request the server rejects. Not found, timeouts, rate limits and the rejections in transientCodes may pass, for example once another process declares the stream or frees capacity.
func permanent(err error) bool {
	for _, target := range []error{
		messaging.ErrClosed, ErrUnsafeBoundConsumer, natsgo.ErrConnectionClosed, natsgo.ErrMaxPayload, natsgo.ErrBadSubject,
		natsgo.ErrBadHeaderMsg, natsgo.ErrPermissionViolation,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	var api *natsjs.APIError
	if errors.As(err, &api) {
		switch api.Code {
		case http.StatusNotFound, http.StatusRequestTimeout, http.StatusTooManyRequests:
			return false
		}
		if slices.Contains(transientCodes, api.ErrorCode) {
			return false
		}
		return api.Code >= http.StatusBadRequest && api.Code < http.StatusInternalServerError
	}
	return false
}

// transientCodes are JetStream rejections that depend on the server's state rather than the request: limits and placement that free up, and a consumer that a concurrent call created or deleted.
var transientCodes = []natsjs.ErrorCode{
	10002, // account resources exceeded
	10005, // no suitable peers
	natsjs.JSErrCodeConsumerNameExists,
	natsjs.JSErrCodeMaximumConsumersLimit,
	10027, // maximum streams reached
	natsjs.JSErrCodeConsumerExists,
	natsjs.JSErrCodeConsumerDoesNotExist,
}
