package client

import (
	"context"
	"errors"
	"fmt"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	kitconnect "github.com/kbukum/gokit/connect"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// CallFailure maps a failed call to peer to a typed error. Outages, timeouts and cancellation are mapped by [MapCallFailure]. Any other failure the peer answered keeps its code, message, reason, violations and retry verdict, with err as its cause; the message is the peer's and is not approved for onward public use. An error that does not decode is a non-retryable Internal failure.
func CallFailure(ctx context.Context, peer string, err error) error {
	if mapped, ok := MapCallFailure(ctx, peer, err); ok {
		return mapped
	}
	remote, decodeErr := kitconnect.DecodeError(err)
	if decodeErr != nil {
		return apperrors.Internal(errors.Join(err, decodeErr))
	}
	return apperrors.New(remote.Code, remote.Message).WithReason(remote.Reason).WithRetryable(remote.Retryable).
		WithRetryAfter(remote.RetryAfter).WithViolations(remote.Violations...).WithCause(err)
}

// Unary makes one unary call to peer with request and returns its response once validator accepts it. A failed call is mapped by [CallFailure]; a missing or invalid response is an Internal failure, since the peer broke its contract. The client's interceptors bound the call.
func Unary[Req, Resp any, PResp interface {
	*Resp
	proto.Message
}](ctx context.Context, peer string, validator protovalidate.Validator, do func(context.Context, *connect.Request[Req]) (*connect.Response[Resp], error), request *Req) (PResp, error) {
	if util.IsNil(ctx) || util.IsNil(validator) || do == nil || request == nil {
		return nil, apperrors.InvalidInput("connect client", "A context, validator, call and request are required")
	}
	response, err := do(ctx, connect.NewRequest(request))
	if err != nil {
		return nil, CallFailure(ctx, peer, err)
	}
	if response == nil || response.Msg == nil {
		return nil, apperrors.Internal(fmt.Errorf("connect client: %s returned no response", peer))
	}
	out := PResp(response.Msg)
	if err := validator.Validate(out); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("connect client: malformed response from %s: %w", peer, err))
	}
	return out, nil
}
