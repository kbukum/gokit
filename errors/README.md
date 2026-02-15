# errors

Structured application error handling with HTTP status codes, error codes, and retryable error support.

## Install

```bash
go get github.com/skillsenselab/gokit
```

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/skillsenselab/gokit/errors"
)

func main() {
    // Create domain-specific errors
    err := errors.NotFound("user", "abc-123")
    fmt.Println(err.Code, err.HTTPStatus) // "NOT_FOUND" 404

    // Chain cause and details
    err = errors.Internal(fmt.Errorf("db timeout")).
        WithDetail("query", "SELECT * FROM users")

    // Check error type
    if appErr, ok := errors.AsAppError(err); ok {
        resp := appErr.ToResponse()
        fmt.Println(resp.Error.Retryable)
    }
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `AppError` | Structured error with code, HTTP status, retryable flag |
| `ErrorCode` | String type for error classification |
| `ErrorResponse` / `ErrorBody` | JSON-serializable error response |
| `New()` | Create custom AppError |
| `NotFound()` / `InvalidInput()` / `Unauthorized()` | Domain error constructors |
| `Internal()` / `DatabaseError()` / `ExternalServiceError()` | Infrastructure error constructors |
| `IsAppError()` / `AsAppError()` | Error type checking |
| `IsRetryableCode()` | Check if an error code is retryable |

---

[⬅ Back to main README](../README.md)
