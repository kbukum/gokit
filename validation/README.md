# validation

Fluent validation builder and struct tag validation with chainable rules and AppError integration.

## Install

```bash
go get github.com/kbukum/gokit
```

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/kbukum/gokit/validation"
)

func main() {
    // Fluent validation
    err := validation.New().
        Required("name", "").
        MaxLength("email", "user@example.com", 255).
        OneOf("role", "editor", []string{"admin", "editor", "viewer"}).
        Validate()
    if err != nil {
        fmt.Println(err) // returns *errors.AppError
    }

    // Struct tag validation
    type User struct {
        Name  string `validate:"required"`
        Email string `validate:"required,email"`
    }
    validator := validation.NewStructValidator()
    if err := validator.Validate(User{}); err != nil {
        fmt.Println(err)
    }
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Validator` | Fluent validation builder collecting field errors |
| `errors.Violation` | Field path, semantic reason, and safe message |
| `New()` | Create a new Validator |
| `Required()` / `RequiredUUID()` / `OptionalUUID()` | Presence checks |
| `MinLength()` / `MaxLength()` / `Range()` / `Min()` / `Max()` | Size/range rules |
| `Before()` / `After()` | Time bound checks |
| `Email()` / `URL()` | Email and URL format checks |
| `Pattern()` / `OneOf()` / `Custom()` | Pattern, enum, and custom rules |
| `NewStructValidator()` | Build once and inject; validates structs using `validate` tags |
| `ValidateUUID()` | Parse and validate UUID string |

Violations use `REQUIRED`, `INVALID_FORMAT`, `OUT_OF_RANGE`, or `INVALID_VALUE`, not validator-specific IDs. Struct paths use JSON names, nested dots, and repeated indexes. RPC paths use protobuf names and require descriptor-aware client translation. Invalid validator inputs and invalid regular expressions are internal evaluation failures with retained causes, not mistakes attributed to the user.

---

[⬅ Back to main README](../README.md)
