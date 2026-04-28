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
	"net/http"

	"github.com/kbukum/gokit/errors"
)

func main() {
	// Create a structured error
	err := errors.NotFound("user", "abc-123")

	fmt.Println(err.Code)       // "NOT_FOUND"
	fmt.Println(err.HTTPStatus) // 404
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
| `EXTERNAL_SERVICE_ERROR` | 500 | Yes |
| `SERVICE_UNAVAILABLE` | 503 | Yes |
| `CONNECTION_FAILED` | 502 | Yes |
| `TIMEOUT` | 504 | Yes |
| `RATE_LIMITED` | 429 | Yes |
| `CANCELED` | 499 | No |

### Builder Methods

- `WithCause(err error)`: Chains an underlying error.
- `WithDetail(key string, value any)`: Adds a single piece of metadata.
- `WithDetails(map[string]any)`: Merges multiple pieces of metadata.

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
fmt.Println(errors.Unwrap(appErr) == cause) // true
```

### Retry Logic

The `Retryable` flag is automatically set based on the `ErrorCode`. This can be used by middleware or clients to implement backoff and retry strategies.

```go
if appErr, ok := errors.AsAppError(err); ok && appErr.Retryable {
    // Implement retry logic
}
```

## Testing

To run the module tests:

```bash
cd errors
go test -race ./...
```

## Contributing

Please refer to the root [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.
