package client

import (
	"context"
	"errors"

	kitconnect "github.com/kbukum/gokit/connect"
	apperrors "github.com/kbukum/gokit/errors"
)

// ReasonUnavailable identifies a peer outage mapped by [MapCallFailure].
const ReasonUnavailable = "PEER_UNAVAILABLE"

// MapCallFailure maps an [IsUnavailable] failure to a local error. Caller cancellation and deadlines retain their causes. Peer outages become ServiceUnavailable with [ReasonUnavailable]; received RPC retry verdicts and minimum delays are preserved, but remote messages are not approved for public use. Malformed RPC details become a non-retryable Internal failure preserving the decode error. Other outcomes return (nil, false); decode non-outage wire errors with connect.DecodeError.
func MapCallFailure(ctx context.Context, peer string, err error) (*apperrors.AppError, bool) {
	if !IsUnavailable(err) {
		return nil, false
	}
	// One read, so a deadline passing mid-check cannot split the classification.
	ended := ctx.Err()
	switch {
	case errors.Is(ended, context.DeadlineExceeded):
		return apperrors.Timeout(peer).WithCause(errors.Join(err, context.Cause(ctx))), true
	case ended != nil:
		return apperrors.Canceled(peer).WithCause(errors.Join(err, context.Cause(ctx))), true
	}
	failure := apperrors.ServiceUnavailable(peer).WithReason(ReasonUnavailable).WithCause(err)
	if IsTransportFailure(err) || errors.Is(err, ErrFirstMessageTimeout) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return failure, true
	}
	remote, decodeErr := kitconnect.DecodeError(err)
	if decodeErr != nil {
		return apperrors.Internal(errors.Join(err, decodeErr)), true
	}
	return failure.WithRetryAfter(remote.RetryAfter).WithRetryable(remote.Retryable), true
}
