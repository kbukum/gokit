package schema

// SchemaDocument is an owned JSON Schema document that has passed structural
// limit checks. It carries the raw schema tree and the limits it was checked
// against, so downstream consumers can reuse a validated schema without
// re-inspecting it.
type SchemaDocument struct {
	value  JSON
	limits ValidationLimits
}

// NewSchemaDocument creates a schema document with the default structural limits.
func NewSchemaDocument(value JSON) (*SchemaDocument, error) {
	return NewSchemaDocumentWithLimits(value, DefaultLimits())
}

// NewSchemaDocumentWithLimits creates a schema document with custom structural limits.
func NewSchemaDocumentWithLimits(value JSON, limits ValidationLimits) (*SchemaDocument, error) {
	if value != nil {
		if err := limits.check("schema", value); err != nil {
			return nil, err
		}
	}
	return &SchemaDocument{value: value, limits: limits}, nil
}

// AsJSON returns the raw JSON schema tree. The returned map is the document's
// backing tree; callers must not mutate it.
func (d *SchemaDocument) AsJSON() JSON { return d.value }

// IntoJSON returns the raw JSON schema tree and detaches it from the document,
// leaving the document empty.
func (d *SchemaDocument) IntoJSON() JSON {
	value := d.value
	d.value = nil
	return value
}

// Compile returns a reusable compiled validator for this document using the
// limits the document was created with.
func (d *SchemaDocument) Compile() (*CompiledSchema, error) {
	return CompileWithLimits(d.value, d.limits)
}

// GenerateDocument generates a JSON Schema from a Go type and wraps it in a
// SchemaDocument checked against the default structural limits.
func GenerateDocument[T any](opts ...Option) (*SchemaDocument, error) {
	return NewSchemaDocument(Generate[T](opts...))
}

// GenerateDocumentWithLimits is like GenerateDocument but applies caller-supplied
// structural limits when checking the generated schema.
func GenerateDocumentWithLimits[T any](limits ValidationLimits, opts ...Option) (*SchemaDocument, error) {
	return NewSchemaDocumentWithLimits(Generate[T](opts...), limits)
}
