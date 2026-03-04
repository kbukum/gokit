package repository

// Option configures a Repository.
type Option func(*repoConfig)

type repoConfig struct {
	idField string
}

// WithIDField overrides the default ID column name ("id").
func WithIDField(field string) Option {
	return func(c *repoConfig) {
		c.idField = field
	}
}

func applyOptions(opts []Option) repoConfig {
	cfg := repoConfig{idField: "id"}
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}
