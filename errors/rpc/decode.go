package rpc

import (
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"

	apperrors "github.com/kbukum/gokit/errors"
)

// Decode preserves enclosing protocol identity and applies recognized details independently of order. Unknown detail types are extensions; malformed known details are errors.
func Decode(wire *statuspb.Status, cause error) (*Error, error) {
	if wire == nil {
		return nil, nil //nolint:nilnil // Nil input represents the absence of a failure.
	}
	if wire.Code <= 0 || wire.Code > 16 {
		return nil, &DecodeError{Detail: "invalid failure status", Cause: cause}
	}
	rpcCode := apperrors.RPCCode(wire.Code)
	code := apperrors.ErrorCodeForRPC(rpcCode)
	result := &Error{Code: code, RPCCode: rpcCode, Message: wire.Message, Retryable: apperrors.IsRetryableCode(code), cause: cause}
	var identity *errdetails.ErrorInfo
	var retry *errdetails.RetryInfo
	var badRequest *errdetails.BadRequest
	for _, detail := range wire.Details {
		if detail == nil {
			return nil, &DecodeError{Detail: "nil detail", Cause: cause}
		}
		var msg proto.Message
		switch {
		case detail.MessageIs(&errdetails.ErrorInfo{}):
			msg = &errdetails.ErrorInfo{}
		case detail.MessageIs(&errdetails.BadRequest{}):
			msg = &errdetails.BadRequest{}
		case detail.MessageIs(&errdetails.RetryInfo{}):
			msg = &errdetails.RetryInfo{}
		default:
			continue
		}
		if err := detail.UnmarshalTo(msg); err != nil {
			return nil, &DecodeError{Detail: "malformed standard detail", Cause: err}
		}
		switch d := msg.(type) {
		case *errdetails.ErrorInfo:
			if d.Domain != Domain {
				continue
			}
			if identity != nil {
				return nil, &DecodeError{Detail: "duplicate application identity", Cause: cause}
			}
			identity = d
		case *errdetails.RetryInfo:
			if retry != nil {
				return nil, &DecodeError{Detail: "duplicate retry hint", Cause: cause}
			}
			retry = d
		case *errdetails.BadRequest:
			if badRequest != nil {
				return nil, &DecodeError{Detail: "duplicate field violations", Cause: cause}
			}
			badRequest = d
		}
	}
	if retry != nil {
		delay := retry.GetRetryDelay()
		if delay != nil {
			if err := delay.CheckValid(); err != nil || delay.Seconds < 0 || delay.Nanos < 0 || delay.Seconds > int64(time.Duration(1<<63-1)/time.Second) {
				return nil, &DecodeError{Detail: "invalid retry delay", Cause: cause}
			}
			// AsDuration saturates overflow; reject rather than silently changing the minimum.
			duration := delay.AsDuration()
			if int64(duration/time.Second) != delay.Seconds || int64(duration%time.Second) != int64(delay.Nanos) {
				return nil, &DecodeError{Detail: "retry delay overflows duration", Cause: cause}
			}
			result.RetryAfter = duration
		}
		result.Retryable = true
	}
	if identity != nil && apperrors.IsKnownCode(apperrors.ErrorCode(identity.Reason)) {
		exact := apperrors.ErrorCode(identity.Reason)
		if apperrors.RPCCodeFor(exact) != rpcCode {
			return nil, &DecodeError{Detail: "application identity contradicts status", Cause: cause}
		}
		value := identity.Metadata["retryable"]
		if value != "true" && value != "false" {
			return nil, &DecodeError{Detail: "missing or invalid retry verdict", Cause: cause}
		}
		result.Code = exact
		result.Retryable = value == "true"
		result.Reason = identity.Metadata["reason"]
		if result.Reason != "" && !apperrors.ValidReason(result.Reason) {
			return nil, &DecodeError{Detail: "invalid application reason", Cause: cause}
		}
		result.TraceID = identity.Metadata["traceId"]
		if !result.Retryable {
			result.RetryAfter = 0
		}
	}
	if badRequest != nil {
		for _, v := range badRequest.FieldViolations {
			if v == nil || (v.Reason != "" && !apperrors.ValidReason(v.Reason)) {
				return nil, &DecodeError{Detail: "invalid field violation", Cause: cause}
			}
			reason := apperrors.ViolationReason(v.Reason)
			if reason == "" {
				reason = apperrors.ViolationInvalidValue
			}
			result.Violations = append(result.Violations, apperrors.Violation{Field: v.Field, Reason: reason, Message: v.Description})
		}
	}
	return result, nil
}
