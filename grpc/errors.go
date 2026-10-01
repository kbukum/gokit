package grpc

import (
	"time"

	"google.golang.org/grpc/status"

	apperrors "github.com/kbukum/gokit/errors"
	errorrpc "github.com/kbukum/gokit/errors/rpc"
)

// AppErrorToStatus encodes an application failure through the shared RPC contract.
func AppErrorToStatus(appErr *apperrors.AppError, service string) (*status.Status, error) {
	wire, err := errorrpc.Encode(appErr, service)
	if err != nil || wire == nil {
		return nil, err
	}
	return status.FromProto(wire), nil
}

// DecodeError decodes a remote gRPC failure without approving its message for onward public serialization.
func DecodeError(err error) (*errorrpc.Error, error) {
	if err == nil {
		return nil, nil //nolint:nilnil // Nil input represents the absence of a failure.
	}
	wire, ok := status.FromError(err)
	if !ok {
		return nil, &errorrpc.DecodeError{Detail: "not a gRPC status", Cause: err}
	}
	return errorrpc.Decode(wire.Proto(), err)
}

// IsRetryable reports the remote transient-failure hint. Malformed failures are not retried.
func IsRetryable(err error) bool {
	remote, decodeErr := DecodeError(err)
	return decodeErr == nil && remote != nil && remote.Retryable
}

// RetryDelay returns the server's minimum retry delay. Decoding errors must be surfaced by the retry owner.
func RetryDelay(err error) (time.Duration, error) {
	remote, decodeErr := DecodeError(err)
	if decodeErr != nil || remote == nil {
		return 0, decodeErr
	}
	return remote.RetryAfter, nil
}
