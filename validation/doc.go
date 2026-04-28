// Package validation provides input validation utilities for gokit handlers.
//
// It supports both struct tag validation (using the validator library) and
// programmatic validation with error collection. Struct tag validation is
// recommended for command/query handlers.
//
// # Struct Tag Validation
//
//	type CreateUserCmd struct {
//	    Name  string `validate:"required,min=2"`
//	    Email string `validate:"required,email"`
//	}
//	err := validation.ValidateStruct(cmd)
//
// # Programmatic Validation
//
//	v := validation.New()
//	v.Check(name != "", "name", "name is required")
//	err := v.Error()
package validation
