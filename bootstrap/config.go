package bootstrap

import (
	"github.com/kbukum/gokit/config"
)

// Config is the interface constraint for application configuration types.
// Any struct that embeds config.ServiceConfig (value embedding) automatically
// satisfies this interface via promoted methods.
//
// Example:
//
//	type MyConfig struct {
//	    config.ServiceConfig `yaml:",inline" mapstructure:",squash"`
//	    Database database.Config `yaml:"database" mapstructure:"database"`
//	}
//
//	// MyConfig automatically satisfies Config via promoted methods.
//	app, err := bootstrap.NewApp[*MyConfig](&cfg)
type Config interface {
	GetServiceConfig() *config.ServiceConfig
	ApplyDefaults()
	Validate() error
}
