package rpc

import (
	"fmt"
	"strconv"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	apperrors "github.com/kbukum/gokit/errors"
)

// Domain identifies the kit's application-error contract, not a remote server's trustworthiness.
const Domain = "gokit.dev"

// Encode serializes the shared vocabulary. Arbitrary AppError.Details are HTTP-only extensions.
func Encode(err *apperrors.AppError, service string) (*statuspb.Status, error) {
	if err == nil {
		return nil, nil //nolint:nilnil // Nil input represents the absence of a failure.
	}
	if validationErr := err.Validate(); validationErr != nil {
		return nil, fmt.Errorf("encode RPC error: %w", validationErr)
	}
	metadata := map[string]string{"retryable": strconv.FormatBool(err.Retryable)}
	for key, value := range map[string]string{"reason": err.Reason, "traceId": err.TraceID, "service": service} {
		if value != "" {
			metadata[key] = value
		}
	}
	details := []proto.Message{&errdetails.ErrorInfo{Domain: Domain, Reason: string(err.Code), Metadata: metadata}}
	if len(err.Violations) > 0 {
		br := &errdetails.BadRequest{}
		for _, v := range err.Violations {
			br.FieldViolations = append(br.FieldViolations, &errdetails.BadRequest_FieldViolation{
				Field: v.Field, Reason: string(v.Reason), Description: v.Message,
			})
		}
		details = append(details, br)
	}
	if err.Retryable {
		details = append(details, &errdetails.RetryInfo{RetryDelay: durationpb.New(err.RetryAfter)})
	}
	wire := &statuspb.Status{Code: int32(apperrors.RPCCodeFor(err.Code)), Message: err.Message}
	for _, detail := range details {
		encoded := &anypb.Any{}
		encodeErr := anypb.MarshalFrom(encoded, detail, proto.MarshalOptions{Deterministic: true})
		if encodeErr != nil {
			return nil, fmt.Errorf("encode RPC error detail: %w", encodeErr)
		}
		wire.Details = append(wire.Details, encoded)
	}
	return wire, nil
}
