package connect

import (
	stderrors "errors"
	"time"

	connectrpc "connectrpc.com/connect"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/types/known/anypb"

	apperrors "github.com/kbukum/gokit/errors"
	errorrpc "github.com/kbukum/gokit/errors/rpc"
)

// ToConnectError encodes an application failure through the shared RPC contract.
func ToConnectError(appErr *apperrors.AppError, service string) (*connectrpc.Error, error) {
	wire, err := errorrpc.Encode(appErr, service)
	if err != nil || wire == nil {
		return nil, err
	}
	result := connectrpc.NewError(connectrpc.Code(wire.Code), stderrors.New(wire.Message))
	for _, detail := range wire.Details {
		d, detailErr := connectrpc.NewErrorDetail(detail)
		if detailErr != nil {
			return nil, detailErr
		}
		result.AddDetail(d)
	}
	return result, nil
}

// DecodeError decodes a remote Connect failure without approving its message for onward public serialization.
func DecodeError(err error) (*errorrpc.Error, error) {
	if err == nil {
		return nil, nil //nolint:nilnil // Nil input represents the absence of a failure.
	}
	var remote *connectrpc.Error
	if !stderrors.As(err, &remote) {
		return nil, &errorrpc.DecodeError{Detail: "not a Connect error", Cause: err}
	}
	wire := &statuspb.Status{Code: int32(remote.Code()), Message: remote.Message()}
	for _, detail := range remote.Details() {
		wire.Details = append(wire.Details, &anypb.Any{TypeUrl: "type.googleapis.com/" + detail.Type(), Value: detail.Bytes()})
	}
	return errorrpc.Decode(wire, err)
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
