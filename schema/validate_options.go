package schema

// ValidationOptions configures schema compilation and value validation.
type ValidationOptions struct {
	// Limits are the structural limits applied to schemas and values before validation.
	Limits ValidationLimits
}

// DefaultOptions returns validation options using the default structural limits.
func DefaultOptions() ValidationOptions {
	return ValidationOptions{Limits: DefaultLimits()}
}

// CompileWithOptions compiles a schema document using explicit validation options.
func CompileWithOptions(s JSON, opts ValidationOptions) (*CompiledSchema, error) {
	return CompileWithLimits(s, opts.Limits)
}

// TryValidate checks a value against the compiled schema, returning a hard error
// when the value violates the compiled structural limits (rather than folding it
// into the ValidationResult). Schema-level violations are caught at compile time.
func (c *CompiledSchema) TryValidate(value any) (ValidationResult, error) {
	if c.schema == nil {
		return ValidationResult{Valid: true}, nil
	}
	if value == nil {
		return invalidResult("value is nil"), nil
	}

	data, err := normalize(value)
	if err != nil {
		return ValidationResult{}, err
	}
	if err := c.limits.check("value", data); err != nil {
		return ValidationResult{}, err
	}

	if err := c.compiled.Validate(data); err != nil {
		return validationResultFromError(err), nil
	}
	return ValidationResult{Valid: true}, nil
}

// ValidateWithOptions validates a value against a schema using explicit options,
// surfacing schema-compile and value structural-limit failures as hard errors.
func ValidateWithOptions(s JSON, value any, opts ValidationOptions) (ValidationResult, error) {
	compiled, err := CompileWithOptions(s, opts)
	if err != nil {
		return ValidationResult{}, err
	}
	return compiled.TryValidate(value)
}

// ValidateStructuredOutput validates structured model output against a JSON Schema.
// It mirrors Validate: invalid schemas and structural-limit violations are folded
// into the ValidationResult so callers can treat them as validation failures.
func ValidateStructuredOutput(s JSON, value any) ValidationResult {
	return Validate(s, value)
}

// ValidateStructuredOutputWithOptions is like ValidateStructuredOutput but applies
// explicit validation options, folding setup failures into the ValidationResult.
func ValidateStructuredOutputWithOptions(s JSON, value any, opts ValidationOptions) ValidationResult {
	result, err := ValidateWithOptions(s, value, opts)
	if err != nil {
		return invalidResult(err.Error())
	}
	return result
}
