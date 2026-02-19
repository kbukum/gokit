// Package config provides configuration loading and validation for gokit
// applications.
//
// It uses Viper to load configuration from files and environment variables,
// supporting multiple formats (YAML, JSON, TOML) and environment-specific
// overrides.
//
// # Usage
//
//	cfg, err := config.Load[MyConfig]("config.yaml")
//
// Environment variables override file values using the APP_ prefix with
// underscore-separated paths (e.g., APP_DATABASE_HOST).
package config
