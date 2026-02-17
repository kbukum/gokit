package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// FileSystem interface for file operations (useful for testing).
type FileSystem interface {
	Exists(path string) bool
	LoadEnv(path string) error
	Getwd() (string, error)
}

// RealFileSystem implements FileSystem using actual file operations.
type RealFileSystem struct{}

func (rfs *RealFileSystem) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (rfs *RealFileSystem) LoadEnv(path string) error {
	return godotenv.Load(path)
}

func (rfs *RealFileSystem) Getwd() (string, error) {
	return os.Getwd()
}

// Resolver handles finding and resolving config and env files.
type Resolver struct {
	FileSystem FileSystem
}

// ResolvedFiles contains the resolved config and env file paths.
type ResolvedFiles struct {
	ConfigFile string
	EnvFile    string
}

// ResolveFiles finds config and env files for a service.
// Returns explicit paths if provided, otherwise searches for them.
func (cr *Resolver) ResolveFiles(serviceName string, opts LoaderConfig) ResolvedFiles {
	resolved := ResolvedFiles{
		ConfigFile: opts.ConfigFile,
		EnvFile:    opts.EnvFile,
	}

	if resolved.ConfigFile == "" {
		resolved.ConfigFile = cr.findConfigFile(serviceName)
	}
	if resolved.EnvFile == "" {
		resolved.EnvFile = cr.findEnvFile(serviceName)
	}

	return resolved
}

// findConfigFile searches for config.yml in standard locations.
func (cr *Resolver) findConfigFile(serviceName string) string {
	shortName := serviceName
	if idx := strings.LastIndex(serviceName, "-"); idx != -1 {
		shortName = serviceName[idx+1:]
	}

	searchPaths := []string{
		fmt.Sprintf("./cmd/%s/config.yml", serviceName),
		fmt.Sprintf("./cmd/%s/config.yml", shortName),
		fmt.Sprintf("../cmd/%s/config.yml", serviceName),
		fmt.Sprintf("../cmd/%s/config.yml", shortName),
		fmt.Sprintf("../../cmd/%s/config.yml", serviceName),
		fmt.Sprintf("../../cmd/%s/config.yml", shortName),
		"./config/config.yml",
		"../config/config.yml",
		"./config.yml",
	}

	for _, path := range searchPaths {
		if cr.FileSystem.Exists(path) {
			return path
		}
	}
	return ""
}

// findEnvFile searches for .env files in standard locations.
func (cr *Resolver) findEnvFile(serviceName string) string {
	shortName := serviceName
	if idx := strings.LastIndex(serviceName, "-"); idx != -1 {
		shortName = serviceName[idx+1:]
	}

	envFiles := []string{
		fmt.Sprintf(".env.%s", serviceName),
		".env",
	}

	searchPaths := buildEnvSearchPaths(serviceName, "")
	if shortName != serviceName {
		searchPaths = append(searchPaths, buildEnvSearchPaths(shortName, "")...)
	}

	for _, envFile := range envFiles {
		for _, basePath := range searchPaths {
			var fullPath string
			if basePath == "" {
				fullPath = envFile
			} else {
				fullPath = fmt.Sprintf("%s/%s", basePath, envFile)
			}
			if cr.FileSystem.Exists(fullPath) {
				return fullPath
			}
		}
	}
	return ""
}

// LoaderConfig holds dependencies and optional file overrides.
type LoaderConfig struct {
	FileSystem FileSystem
	ConfigFile string // Direct config file path (optional)
	EnvFile    string // Direct env file path (optional)
}

// LoaderOption is a functional option for LoadConfig.
type LoaderOption func(*LoaderConfig)

// WithFileSystem sets a custom filesystem for the loader.
func WithFileSystem(fs FileSystem) LoaderOption {
	return func(lc *LoaderConfig) { lc.FileSystem = fs }
}

// WithConfigFile sets an explicit config file path.
func WithConfigFile(path string) LoaderOption {
	return func(lc *LoaderConfig) { lc.ConfigFile = path }
}

// WithEnvFile sets an explicit .env file path.
func WithEnvFile(path string) LoaderOption {
	return func(lc *LoaderConfig) { lc.EnvFile = path }
}

// LoadConfig loads configuration for a service into the provided cfg struct.
// It searches for config.yml and .env files in standard locations, binds
// environment variables, and unmarshals the result into cfg.
func LoadConfig(serviceName string, cfg interface{}, opts ...LoaderOption) error {
	var lc LoaderConfig
	for _, opt := range opts {
		opt(&lc)
	}
	if lc.FileSystem == nil {
		lc.FileSystem = &RealFileSystem{}
	}

	resolver := &Resolver{FileSystem: lc.FileSystem}
	files := resolver.ResolveFiles(serviceName, lc)

	return loadFromResolvedFiles(serviceName, cfg, files, lc.FileSystem)
}

// loadFromResolvedFiles loads configuration from specific files.
func loadFromResolvedFiles(serviceName string, cfg interface{}, files ResolvedFiles, fs FileSystem) error {
	v := viper.New()

	// 1. Load YAML config first (base configuration)
	if files.ConfigFile != "" && fs.Exists(files.ConfigFile) {
		v.SetConfigFile(files.ConfigFile)
		if err := v.ReadInConfig(); err != nil {
			fmt.Printf("[config] warning: failed to load config file %s: %v\n", files.ConfigFile, err)
		}
	}

	// 2. Enable automatic environment variable reading
	v.AutomaticEnv()
	autoBindEnvVars(v)

	// 3. Load .env file
	if files.EnvFile != "" && fs.Exists(files.EnvFile) {
		if err := fs.LoadEnv(files.EnvFile); err != nil {
			fmt.Printf("[config] warning: failed to load .env file %s: %v\n", files.EnvFile, err)
		} else {
			// Re-bind env vars after loading .env to pick up new variables
			autoBindEnvVars(v)
		}
	}

	// 4. Unmarshal into config struct
	if err := v.Unmarshal(cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config for service %s: %w", serviceName, err)
	}

	return nil
}

// buildEnvSearchPaths creates a list of paths to search for .env files.
func buildEnvSearchPaths(serviceName, envFileName string) []string {
	primaryPaths := pathsByPrefix(fmt.Sprintf("cmd/%s", serviceName), envFileName)
	configServicePaths := pathsByPrefix(fmt.Sprintf("config/%s", serviceName), envFileName)
	configPaths := pathsByPrefix("config", envFileName)
	rootPaths := pathsByPrefix("", envFileName)

	paths := make([]string, 0, len(primaryPaths)+len(configServicePaths)+len(configPaths)+len(rootPaths))
	paths = append(paths, primaryPaths...)
	paths = append(paths, configServicePaths...)
	paths = append(paths, configPaths...)
	paths = append(paths, rootPaths...)

	return paths
}

func pathsByPrefix(path, fileName string) []string {
	if path == "" {
		return []string{
			fmt.Sprintf("./%s", fileName),
			fmt.Sprintf("../%s", fileName),
			fmt.Sprintf("../../%s", fileName),
			fileName,
		}
	}
	return []string{
		fmt.Sprintf("./%s/%s", path, fileName),
		fmt.Sprintf("../%s/%s", path, fileName),
		fmt.Sprintf("../../%s/%s", path, fileName),
	}
}

// autoBindEnvVars automatically binds environment variables to Viper
// by converting UPPER_CASE_WITH_UNDERSCORES to multiple possible nested key formats.
func autoBindEnvVars(v *viper.Viper) {
	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) != 2 {
			continue
		}

		key := pair[0]
		value := pair[1]

		variants := generateEnvKeyVariants(key)
		for _, variant := range variants {
			v.Set(variant, value)
		}
	}
}

// generateEnvKeyVariants creates all possible key variants for environment variable binding.
// Examples:
//
//	AUTH_JWT_SECRET -> [auth_jwt_secret, auth.jwt.secret, auth.jwt_secret]
//	HTTP_CORS_ALLOWED_ORIGINS -> [http_cors_allowed_origins, http.cors.allowed.origins, http.cors_allowed_origins, ...]
func generateEnvKeyVariants(envKey string) []string {
	lowerKey := strings.ToLower(envKey)
	parts := strings.Split(lowerKey, "_")

	if len(parts) <= 1 {
		return []string{lowerKey}
	}

	variants := []string{
		lowerKey,
		strings.ReplaceAll(lowerKey, "_", "."),
	}

	// Generate progressive nesting patterns
	for i := 1; i < len(parts); i++ {
		prefix := strings.Join(parts[:i], ".")
		suffix := strings.Join(parts[i:], "_")
		variants = append(variants, prefix+"."+suffix)
	}

	for i := 2; i <= len(parts); i++ {
		prefix := strings.Join(parts[:i-1], ".")
		suffix := strings.Join(parts[i-1:], "_")
		if i < len(parts) {
			variants = append(variants, prefix+"."+suffix)
		}
	}

	if len(parts) >= 3 {
		prefix := strings.Join(parts[:len(parts)-1], ".")
		lastPart := parts[len(parts)-1]
		variants = append(variants, prefix+"."+lastPart)
	}

	return removeDuplicates(variants)
}

// removeDuplicates removes duplicate strings from a slice.
func removeDuplicates(items []string) []string {
	seen := make(map[string]bool, len(items))
	result := make([]string, 0, len(items))

	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}
