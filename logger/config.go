package logger

import "fmt"

// Config contains logging configuration.
type Config struct {
	Level      string `yaml:"level" mapstructure:"level"`
	Format     string `yaml:"format" mapstructure:"format"`
	Output     string `yaml:"output" mapstructure:"output"`
	NoColor    bool   `yaml:"no_color" mapstructure:"no_color"`
	Timestamp  bool   `yaml:"timestamp" mapstructure:"timestamp"`
	Caller     bool   `yaml:"caller" mapstructure:"caller"`
	Stacktrace bool   `yaml:"stacktrace" mapstructure:"stacktrace"`
	MaxSize    int    `yaml:"max_size" mapstructure:"max_size"`       // megabytes
	MaxBackups int    `yaml:"max_backups" mapstructure:"max_backups"` // number of backups
	MaxAge     int    `yaml:"max_age" mapstructure:"max_age"`         // days
	Compress   bool   `yaml:"compress" mapstructure:"compress"`
	LocalTime  bool   `yaml:"local_time" mapstructure:"local_time"`
}

// ApplyDefaults applies default values to logging configuration.
func (c *Config) ApplyDefaults() {
	if c.Level == "" {
		c.Level = "info"
	}
	if c.Format == "" {
		c.Format = "console"
	}
	if c.Output == "" {
		c.Output = "stdout"
	}
	if c.MaxSize == 0 {
		c.MaxSize = 100
	}
	if c.MaxBackups == 0 {
		c.MaxBackups = 3
	}
	if c.MaxAge == 0 {
		c.MaxAge = 28
	}
	c.Timestamp = true
}

// Validate validates logging configuration.
func (c *Config) Validate() error {
	validLevels := []string{"debug", "info", "warn", "error", "fatal", "trace"}
	if !contains(validLevels, c.Level) {
		return fmt.Errorf("logging.level must be one of %v (got: %s)", validLevels, c.Level)
	}
	validFormats := []string{"json", "console", "text"}
	if !contains(validFormats, c.Format) {
		return fmt.Errorf("logging.format must be one of %v (got: %s)", validFormats, c.Format)
	}
	return nil
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
