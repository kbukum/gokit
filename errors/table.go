package errors

import "net/http"

// RPCCode is the canonical RPC status code shared by Connect and gRPC. Its
// values mirror the gRPC/Connect numeric code space exactly, so a transport
// adapter converts with a single numeric cast (connect.Code(rpc) or
// codes.Code(rpc)) and never needs its own code switch. Keeping the vocabulary
// here lets the errors package own the one code table without importing any
// transport package.
type RPCCode uint32

// Canonical RPC status codes (gRPC/Connect numbering).
const (
	RPCCodeOK                 RPCCode = 0
	RPCCodeCanceled           RPCCode = 1
	RPCCodeUnknown            RPCCode = 2
	RPCCodeInvalidArgument    RPCCode = 3
	RPCCodeDeadlineExceeded   RPCCode = 4
	RPCCodeNotFound           RPCCode = 5
	RPCCodeAlreadyExists      RPCCode = 6
	RPCCodePermissionDenied   RPCCode = 7
	RPCCodeResourceExhausted  RPCCode = 8
	RPCCodeFailedPrecondition RPCCode = 9
	RPCCodeAborted            RPCCode = 10
	RPCCodeOutOfRange         RPCCode = 11
	RPCCodeUnimplemented      RPCCode = 12
	RPCCodeInternal           RPCCode = 13
	RPCCodeUnavailable        RPCCode = 14
	RPCCodeDataLoss           RPCCode = 15
	RPCCodeUnauthenticated    RPCCode = 16
)

// codeSpec binds an application category to its REST status, RPC status and default transient-failure hint.
type codeSpec struct {
	httpStatus int
	rpcCode    RPCCode
	retryable  bool
}

// codeTable is the authoritative code → {HTTP status, RPC code, retryable} map.
// It is the only place these relationships are defined; connect, grpc, and the
// problem+json encoder all read it rather than keeping parallel switches.
var codeTable = map[ErrorCode]codeSpec{
	ErrCodeServiceUnavailable: {http.StatusServiceUnavailable, RPCCodeUnavailable, true},
	ErrCodeConnectionFailed:   {http.StatusBadGateway, RPCCodeUnavailable, true},
	ErrCodeTimeout:            {http.StatusGatewayTimeout, RPCCodeDeadlineExceeded, true},
	ErrCodeRateLimited:        {http.StatusTooManyRequests, RPCCodeResourceExhausted, true},
	ErrCodeExternalService:    {http.StatusInternalServerError, RPCCodeInternal, false},

	ErrCodeNotFound:      {http.StatusNotFound, RPCCodeNotFound, false},
	ErrCodeAlreadyExists: {http.StatusConflict, RPCCodeAlreadyExists, false},
	ErrCodeConflict:      {http.StatusConflict, RPCCodeFailedPrecondition, false},

	ErrCodeInvalidInput:  {http.StatusUnprocessableEntity, RPCCodeInvalidArgument, false},
	ErrCodeMissingField:  {http.StatusUnprocessableEntity, RPCCodeInvalidArgument, false},
	ErrCodeInvalidFormat: {http.StatusUnprocessableEntity, RPCCodeInvalidArgument, false},

	ErrCodeUnauthorized: {http.StatusUnauthorized, RPCCodeUnauthenticated, false},
	ErrCodeTokenExpired: {http.StatusUnauthorized, RPCCodeUnauthenticated, false},
	ErrCodeInvalidToken: {http.StatusUnauthorized, RPCCodeUnauthenticated, false},
	ErrCodeForbidden:    {http.StatusForbidden, RPCCodePermissionDenied, false},

	ErrCodeInternal:      {http.StatusInternalServerError, RPCCodeInternal, false},
	ErrCodeDatabaseError: {http.StatusInternalServerError, RPCCodeInternal, false},

	ErrCodeCanceled: {http.StatusRequestTimeout, RPCCodeCanceled, false},
}

// unknownSpec is used for any code not present in codeTable: a safe, internal,
// non-retryable default rather than a silent zero value.
var unknownSpec = codeSpec{http.StatusInternalServerError, RPCCodeInternal, false}

func specFor(code ErrorCode) codeSpec {
	if s, ok := codeTable[code]; ok {
		return s
	}
	return unknownSpec
}

// HTTPStatusFor returns the canonical HTTP status for a code from the one table.
func HTTPStatusFor(code ErrorCode) int { return specFor(code).httpStatus }

// RPCCodeFor returns the canonical RPC code for a code from the one table.
func RPCCodeFor(code ErrorCode) RPCCode { return specFor(code).rpcCode }

// IsRetryableCode reports whether a code is retryable, from the one table.
func IsRetryableCode(code ErrorCode) bool { return specFor(code).retryable }

// IsKnownCode reports whether the kit defines the application category.
func IsKnownCode(code ErrorCode) bool {
	_, ok := codeTable[code]
	return ok
}

// errorCodeByRPC is the reverse of the RPC column, used to reconstruct an
// ErrorCode from a wire status when no richer detail is present. It maps each
// canonical RPC code to the representative ErrorCode that produces it.
var errorCodeByRPC = map[RPCCode]ErrorCode{
	RPCCodeCanceled:           ErrCodeCanceled,
	RPCCodeInvalidArgument:    ErrCodeInvalidInput,
	RPCCodeDeadlineExceeded:   ErrCodeTimeout,
	RPCCodeNotFound:           ErrCodeNotFound,
	RPCCodeAlreadyExists:      ErrCodeAlreadyExists,
	RPCCodePermissionDenied:   ErrCodeForbidden,
	RPCCodeResourceExhausted:  ErrCodeRateLimited,
	RPCCodeFailedPrecondition: ErrCodeConflict,
	RPCCodeUnauthenticated:    ErrCodeUnauthorized,
	RPCCodeUnavailable:        ErrCodeServiceUnavailable,
	RPCCodeInternal:           ErrCodeInternal,
}

// ErrorCodeForRPC maps a canonical RPC code back to its representative
// ErrorCode. Codes without a distinct mapping (Unknown, Aborted, OutOfRange,
// Unimplemented, DataLoss, OK) collapse to ErrCodeInternal, matching the
// safe-by-default normalization rule.
func ErrorCodeForRPC(code RPCCode) ErrorCode {
	if c, ok := errorCodeByRPC[code]; ok {
		return c
	}
	return ErrCodeInternal
}
