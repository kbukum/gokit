package tool

// Filter returns definitions matching the given options.
func (r *Registry) Filter(opts ...FilterOption) []Definition {
	cfg := &filterConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	var result []Definition
	r.inner.Each(func(_ string, t Callable) {
		def := t.Definition()
		if matchesFilter(def, cfg) {
			result = append(result, def)
		}
	})
	return result
}

// FilterOption configures tool filtering.
type FilterOption func(*filterConfig)

type filterConfig struct {
	category      string
	tags          []string
	executionHint *ExecutionHint
}

// WithCategory filters tools by category annotation.
func WithCategory(cat string) FilterOption {
	return func(c *filterConfig) { c.category = cat }
}

// WithTags filters tools that have all specified tags.
func WithTags(tags ...string) FilterOption {
	return func(c *filterConfig) { c.tags = tags }
}

// WithExecutionHint filters tools by resolved execution hint annotation.
func WithExecutionHint(hint ExecutionHint) FilterOption {
	return func(c *filterConfig) { c.executionHint = &hint }
}

func matchesFilter(def Definition, cfg *filterConfig) bool {
	if cfg.category != "" {
		if def.Annotations.Category != cfg.category {
			return false
		}
	}
	if cfg.executionHint != nil {
		if def.Annotations.ExecutionHint.Resolved() != cfg.executionHint.Resolved() {
			return false
		}
	}
	if len(cfg.tags) > 0 {
		tagSet := make(map[string]bool, len(def.Annotations.Tags))
		for _, t := range def.Annotations.Tags {
			tagSet[t] = true
		}
		for _, required := range cfg.tags {
			if !tagSet[required] {
				return false
			}
		}
	}
	return true
}
