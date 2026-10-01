# gokit/errors

Unified application error handling with HTTP status codes, error codes, and retryable error support.

## Overview

The `errors` module provides a structured approach to error handling across Go microservices. It moves away from simple string-based errors towards a machine-readable `AppError` type that includes semantic error codes, HTTP status mappings, and metadata.

This design follows best practices such as RFC 9457 (Problem Details for HTTP APIs) and Google AIP-193. It allows services to communicate clearly about what went wrong, whether the client should retry, and provides structured details that can be easily parsed by frontend applications or observability tools.

## Installation

```bash
go get github.com/kbukum/gokit/errors
```

## Quick Start

```go
package main

import (
	"fmt"

	"github.com/kbukum/gokit/errors"
)

func main() {
	// Create a structured error
	err := errors.NotFound("user", "abc-123")

	fmt.Println(err.Code)       // "NOT_FOUND"
	fmt.Println(err.HTTPStatus()) // 404
	fmt.Println(err.Retryable)  // false

	// Check if it's an AppError and convert to response
	if appErr, ok := errors.AsAppError(err); ok {
		resp := appErr.ToProblemDetail()
		// Send resp as JSON to the client
		fmt.Printf("Response: %+v\n", resp)
	}
}
```

## Configuration

This module does not require external configuration. It uses a predefined set of error codes and mapping logic.

## API Reference

### Major Types

| Field | Type | Description |
|-------|------|-------------|
| `AppError` | `struct` | The core error type implementing the `error` interface. |
| `ErrorCode` | `string` | A machine-readable string representing the error category. |
| `ProblemDetail` | `struct` | RFC 9457 Problem Details response type for JSON serialization. |

### Common Constructors

- `NotFound(resource, id string) *AppError`: For missing resources.
- `InvalidInput(field, reason string) *AppError`: For validation failures (HTTP 422).
- `Unauthorized(reason string) *AppError`: For authentication issues.
- `Forbidden(reason string) *AppError`: For permission issues.
- `Internal(cause error) *AppError`: For unexpected server-side errors.
- `DatabaseError(cause error) *AppError`: For database errors (non-retryable).
- `ServiceUnavailable(service string) *AppError`: For temporary outages (retryable).

### Error Codes

The `ErrorCode` type defines all machine-readable error categories:

| Code | HTTP Status | Retryable |
|------|-------------|-----------|
| `NOT_FOUND` | 404 | No |
| `ALREADY_EXISTS` | 409 | No |
| `CONFLICT` | 409 | No |
| `INVALID_INPUT` | 422 | No |
| `MISSING_FIELD` | 422 | No |
| `INVALID_FORMAT` | 422 | No |
| `UNAUTHORIZED` | 401 | No |
| `FORBIDDEN` | 403 | No |
| `TOKEN_EXPIRED` | 401 | No |
| `INVALID_TOKEN` | 401 | No |
| `INTERNAL_ERROR` | 500 | No |
| `DATABASE_ERROR` | 500 | No |
| `EXTERNAL_SERVICE_ERROR` | 500 | No |
| `SERVICE_UNAVAILABLE` | 503 | Yes |
| `CONNECTION_FAILED` | 502 | Yes |
| `TIMEOUT` | 504 | Yes |
| `RATE_LIMITED` | 429 | Yes |
| `CANCELED` | 408 | No |

### Builder Methods

- `WithCause(err error)`: Chains an underlying error.
- `WithDetail(key string, value any)`: Adds a single piece of metadata.
- `WithDetails(map[string]any)`: Merges multiple pieces of metadata.
- `WithReason(reason string)`: Adds a stable domain reason.
- `WithViolations(...Violation)`: Adds typed field problems with semantic reasons.
- `WithRetryable(bool)`: Explicitly overrides the transient-failure hint; false clears the delay.
- `WithRetryAfter(time.Duration)`: Marks the failure transient with a minimum retry delay.
- `WithTraceID(string)`: Adds safe log correlation.

`New(code, message)` derives REST status from the code; there is no status override. Application reasons and violation reasons are UPPER_SNAKE_CASE with at most 63 characters. Public encoders validate the vocabulary and report invalid values as internal failures. `Normalize` preserves explicit application outcomes before classifying otherwise-untyped context errors.

An `AppError` is a deliberate public response. Its message, violations, and HTTP details must be safe to disclose; put raw OS errors, decoder failures, and other internal diagnostics in `Cause`. Normalization does not sanitize trusted application fields. Malformed upstream responses are external-service failures, not caller validation errors, and are not automatically retryable.

### Shared RPC contract

[`errors/rpc`](rpc) owns protobuf encoding and decoding without importing Connect or gRPC implementations. Both transport adapters consume it. `ErrorInfo.domain` is `gokit.dev`; its `reason` is the application code. Metadata contains the mandatory `retryable` string (`true` or `false`) and optional domain `reason`, `traceId`, and service. `BadRequest` carries semantic field violations. `RetryInfo` carries a minimum delay. Details are decoded independently of their order.

Decoded `rpc.Error` values preserve remote RPC identity and raw remote messages, but are not trusted `AppError` values for onward serialization. Foreign or unknown identities do not replace protocol status. Malformed recognized details produce an explicit decode error. Arbitrary `Details` remain HTTP-only; they are not part of the shared RPC vocabulary.

Connect owns its protocol HTTP mapping, which may differ from the REST table. A transient database or external-service error still has its category's 500 REST status; clients use its explicit retry hint, not a status override.

### RFC 9457 Support

Convert any `AppError` to an [RFC 9457 Problem Details](https://www.rfc-editor.org/rfc/rfc9457) response:

```go
appErr := errors.NotFound("user", "abc-123")
rfc := appErr.ToProblemDetail()
// ProblemDetail{Type, Title, Status, Detail, Instance, Code, Retryable, Details}
```

The `ProblemDetail` struct includes `Type`, `Title`, `Status`, `Detail`, `Instance`, `Code`, `Retryable`, and `Details` fields suitable for direct JSON serialization in HTTP APIs.

## Advanced Usage

### Error Wrapping

`AppError` supports Go 1.13+ error wrapping. You can use `errors.Is` and `errors.As` from the standard library.

```go
cause := fmt.Errorf("connection refused")
appErr := errors.DatabaseError(cause)

// Unwrap works as expected
fmt.Println(appErr.Unwrap() == cause) // true
```

### Retry Logic

`Retryable` starts from the code default and can be explicitly overridden. It describes the failure, not whether repeating an operation is safe. Generic database and external-service errors default to false. Retry owners require operation idempotency, bounded attempts, and one retry loop; server delays are minimums and are never shortened to fit a backoff cap.

```go
// Only after establishing that the failure is transient:
err := errors.DatabaseError(cause).WithRetryAfter(time.Second)
```

## Testing

To run the module tests:

```bash
cd errors
go test -race ./...
```

## Contributing

Please refer to the root [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.
