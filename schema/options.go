package schema

// Option configures JSON Schema generation.
type Option func(*config)

type config struct {
	title           string
	description     string
	inline          bool
	allowAdditional bool
}

func applyOptions(opts []Option) *config {
	cfg := &config{
		inline: true, // default: inline all definitions (no $ref)
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// WithTitle overrides the root schema title.
func WithTitle(title string) Option {
	return func(c *config) { c.title = title }
}

// WithDescription overrides the root schema description.
func WithDescription(desc string) Option {
	return func(c *config) { c.description = desc }
}

// WithDefinitions keeps $defs and uses $ref for nested types instead of inlining all definitions. Default is to inline.
func WithDefinitions() Option {
	return func(c *config) { c.inline = false }
}

// WithAdditionalProperties allows additional properties in the generated schema. Default is strict (no additional properties).
func WithAdditionalProperties() Option {
	return func(c *config) { c.allowAdditional = true }
}
