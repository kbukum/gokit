package huggingface

// DefaultMaxDefinitionBytes bounds how many bytes a tokenizer definition may
// occupy when loaded from a file or reader. Hugging Face tokenizer.json files are
// typically a few megabytes; the cap guards against unbounded reads from an
// untrusted source while comfortably fitting real definitions.
const DefaultMaxDefinitionBytes int64 = 64 << 20

// Config selects a tokenizer definition by local path; it is the config-driven
// registration seam for constructing a counter via [NewCounter].
type Config struct {
	// Path is the local filesystem path to a Hugging Face tokenizer.json file.
	Path string
	// MaxDefinitionBytes optionally overrides [DefaultMaxDefinitionBytes]. A
	// non-positive value selects the default.
	MaxDefinitionBytes int64
}

// maxBytes resolves the effective read bound for the config.
func (c Config) maxBytes() int64 {
	if c.MaxDefinitionBytes > 0 {
		return c.MaxDefinitionBytes
	}
	return DefaultMaxDefinitionBytes
}
