package rpc

import (
	"testing"

	codepb "google.golang.org/genproto/googleapis/rpc/code"

	apperrors "github.com/kbukum/gokit/errors"
)

func TestNeutralCodesMatchPublishedRPCEnum(t *testing.T) {
	t.Parallel()
	pairs := []struct {
		neutral apperrors.RPCCode
		wire    codepb.Code
	}{
		{apperrors.RPCCodeOK, codepb.Code_OK},
		{apperrors.RPCCodeCanceled, codepb.Code_CANCELLED}, //nolint:misspell // Name is defined by the upstream protobuf enum.
		{apperrors.RPCCodeUnknown, codepb.Code_UNKNOWN},
		{apperrors.RPCCodeInvalidArgument, codepb.Code_INVALID_ARGUMENT},
		{apperrors.RPCCodeDeadlineExceeded, codepb.Code_DEADLINE_EXCEEDED},
		{apperrors.RPCCodeNotFound, codepb.Code_NOT_FOUND},
		{apperrors.RPCCodeAlreadyExists, codepb.Code_ALREADY_EXISTS},
		{apperrors.RPCCodePermissionDenied, codepb.Code_PERMISSION_DENIED},
		{apperrors.RPCCodeResourceExhausted, codepb.Code_RESOURCE_EXHAUSTED},
		{apperrors.RPCCodeFailedPrecondition, codepb.Code_FAILED_PRECONDITION},
		{apperrors.RPCCodeAborted, codepb.Code_ABORTED},
		{apperrors.RPCCodeOutOfRange, codepb.Code_OUT_OF_RANGE},
		{apperrors.RPCCodeUnimplemented, codepb.Code_UNIMPLEMENTED},
		{apperrors.RPCCodeInternal, codepb.Code_INTERNAL},
		{apperrors.RPCCodeUnavailable, codepb.Code_UNAVAILABLE},
		{apperrors.RPCCodeDataLoss, codepb.Code_DATA_LOSS},
		{apperrors.RPCCodeUnauthenticated, codepb.Code_UNAUTHENTICATED},
	}
	for _, pair := range pairs {
		if int32(pair.neutral) != int32(pair.wire) {
			t.Errorf("%s numeric mapping differs: %d", pair.wire, pair.neutral)
		}
	}
}
