# validation

Fluent validation builder and struct tag validation with chainable rules and AppError integration.

## Install

```bash
go get github.com/skillsenselab/gokit
```

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/skillsenselab/gokit/validation"
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
    if err := validation.Validate(User{}); err != nil {
        fmt.Println(err)
    }
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Validator` | Fluent validation builder collecting field errors |
| `FieldError` | Individual field validation error |
| `New()` | Create a new Validator |
| `Required()` / `RequiredUUID()` / `OptionalUUID()` | Presence checks |
| `MinLength()` / `MaxLength()` / `Range()` / `Min()` / `Max()` | Size/range rules |
| `Pattern()` / `OneOf()` / `Custom()` | Pattern, enum, and custom rules |
| `Validate(s any)` | Struct tag validation using `validate` tags |
| `ValidateUUID()` | Parse and validate UUID string |

---

[⬅ Back to main README](../README.md)
