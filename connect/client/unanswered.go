package client

import (
	"context"
	"errors"

	apperrors "github.com/kbukum/gokit/errors"
)

// ReasonUnavailable is the reason [Unanswered] gives a peer outage.
const ReasonUnavailable = "PEER_UNAVAILABLE"

// Unanswered maps a failed call made with ctx to a local error when the peer did not answer it, and reports whether it did so. A failure after ctx ended belongs to the caller: Timeout when its deadline passed and Canceled otherwise, both without a cause because they say nothing about the peer. Any other [IsUnavailable] failure is a retryable ServiceUnavailable for peer with [ReasonUnavailable] and err as its cause. A nil error or an error the peer served returns (nil, false); decode it with connect.DecodeError and map it by its code.
func Unanswered(ctx context.Context, peer string, err error) (*apperrors.AppError, bool) {
	if !IsUnavailable(err) {
		return nil, false
	}
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return apperrors.Timeout(peer), true
	case ctx.Err() != nil:
		return apperrors.Canceled(peer), true
	}
	return apperrors.ServiceUnavailable(peer).WithReason(ReasonUnavailable).WithRetryable(true).WithCause(err), true
}
