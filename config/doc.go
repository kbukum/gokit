// Package config provides configuration loading and validation for gokit applications.
//
// It uses Viper to load configuration from files and environment variables,
// supporting multiple formats (YAML, JSON, TOML) and environment-specific overrides.
//
// # Usage
//
//	var cfg ServiceConfig
//	err := config.LoadConfig("my-service", &cfg)
//
// Environment variables override file values: an underscore-separated variable such as
// DATABASE_HOST binds to the nested key database.host. Use [LoadStrict] to reject
// unknown keys instead of ignoring them.
//
// # Watching and sinks
//
// [ConfigWatch] streams [ConfigChange] events as configuration entries are set or removed,
// and a [ConfigSink] ([NewInMemoryConfigSink] or [NewFileConfigSink]) persists runtime
// configuration entries. [AppConfig] is the typed contract an application config satisfies
// by embedding *ServiceConfig and implementing ApplyDefaults and Validate.
package config
