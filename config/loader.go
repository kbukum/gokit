package config

import (
	"errors"
	"fmt"
	iofs "io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"

	"github.com/kbukum/gokit/codec"
	gokitfs "github.com/kbukum/gokit/fs"
	"github.com/kbukum/gokit/logging"
)

// FileSystem interface for file operations (useful for testing).
type FileSystem interface {
	// Exists reports whether path exists. A path that does not exist returns false and a nil error; any other probe
	// failure, such as permission denied, is returned.
	Exists(path string) (bool, error)
	LoadEnv(path string) error
	Getwd() (string, error)
}

// RealFileSystem implements FileSystem using actual file operations.
type RealFileSystem struct{}

func (rfs *RealFileSystem) Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, iofs.ErrNotExist) {
		return false, nil
	}
	return false, err
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
	ConfigFile     string
	ProfileEnvFile string
	EnvFile        string
}

var (
	// ErrFileNotFound reports an explicitly requested config, env or profile file that does not exist.
	ErrFileNotFound = errors.New("config: file not found")
	// ErrInvalidProfile reports a profile name that is not a lowercase slug, so it cannot escape the profile directory.
	ErrInvalidProfile = errors.New("config: invalid profile name")

	profileName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
)

// ResolveFiles finds config and env files for a service. Explicit paths and an explicitly named profile must exist;
// discovered files and an ENVIRONMENT-derived profile are optional.
func (cr *Resolver) ResolveFiles(serviceName string, opts LoaderConfig) (ResolvedFiles, error) {
	resolved := ResolvedFiles{
		ConfigFile: opts.ConfigFile,
		EnvFile:    opts.EnvFile,
	}
	for _, path := range []string{opts.ConfigFile, opts.EnvFile} {
		if path == "" {
			continue
		}
		ok, err := cr.FileSystem.Exists(path)
		if err != nil {
			return ResolvedFiles{}, fmt.Errorf("config: check %q: %w", path, err)
		}
		if !ok {
			return ResolvedFiles{}, fmt.Errorf("%w: %q", ErrFileNotFound, path)
		}
	}

	if !opts.DisableDiscovery {
		var err error
		if resolved.ConfigFile == "" {
			if resolved.ConfigFile, err = cr.firstExisting(configSearchPaths(serviceName)); err != nil {
				return ResolvedFiles{}, err
			}
		}
		if resolved.EnvFile == "" {
			if resolved.EnvFile, err = cr.firstExisting(envSearchPaths(serviceName)); err != nil {
				return ResolvedFiles{}, err
			}
		}
	}

	if opts.ProfileEnabled {
		profile, required := opts.Profile, opts.Profile != ""
		if !required {
			profile = os.Getenv("ENVIRONMENT")
		}
		file, err := cr.findProfileEnvFile(profile, opts.ProfileDir, opts.DisableDiscovery)
		if err != nil {
			return ResolvedFiles{}, err
		}
		if required && file == "" {
			return ResolvedFiles{}, fmt.Errorf("%w: profile %q", ErrFileNotFound, profile)
		}
		resolved.ProfileEnvFile = file
	}

	return resolved, nil
}

// firstExisting returns the first path that exists, or "" when none do. Probe failures other than "not found" are
// returned rather than treated as absence.
func (cr *Resolver) firstExisting(paths []string) (string, error) {
	for _, path := range paths {
		ok, err := cr.FileSystem.Exists(path)
		if err != nil {
			return "", fmt.Errorf("config: check %q: %w", path, err)
		}
		if ok {
			return path, nil
		}
	}
	return "", nil
}

// configSearchPaths lists the standard config.yml locations in search order.
func configSearchPaths(serviceName string) []string {
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
	return searchPaths
}

// envSearchPaths lists the standard .env locations in search order.
func envSearchPaths(serviceName string) []string {
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

	var paths []string
	for _, envFile := range envFiles {
		for _, basePath := range searchPaths {
			if basePath == "" {
				paths = append(paths, envFile)
			} else {
				paths = append(paths, fmt.Sprintf("%s/%s", basePath, envFile))
			}
		}
	}
	return paths
}

// findProfileEnvFile looks for <profile>.env in dir when set, otherwise in the standard relative locations unless
// discovery is disabled.
func (cr *Resolver) findProfileEnvFile(profile, dir string, noDiscovery bool) (string, error) {
	if profile == "" {
		return "", nil
	}
	if !profileName.MatchString(profile) {
		return "", fmt.Errorf("%w: %q", ErrInvalidProfile, profile)
	}
	var searchPaths []string
	switch {
	case dir != "":
		searchPaths = []string{filepath.Join(dir, profile+".env")}
	case !noDiscovery:
		searchPaths = []string{
			fmt.Sprintf("./config/profiles/%s.env", profile),
			fmt.Sprintf("../config/profiles/%s.env", profile),
			fmt.Sprintf("../../config/profiles/%s.env", profile),
		}
	}
	return cr.firstExisting(searchPaths)
}

// LoaderConfig holds dependencies and optional file overrides.
type LoaderConfig struct {
	FileSystem     FileSystem
	ConfigFile     string // Direct config file path (optional)
	EnvFile        string // Direct env file path (optional)
	Profile        string // Profile name (e.g., "development", "docker", "staging")
	ProfileEnabled bool   // Whether profile loading was explicitly enabled
	ProfileDir     string // Only directory searched for <profile>.env (optional)
	// DisableDiscovery skips the relative config.yml, .env and profile search so only explicit inputs load.
	DisableDiscovery bool
	WarningLogger    WarningFunc
}

// LoaderOption is a functional option for LoadConfig.
type LoaderOption func(*LoaderConfig)

// WarningFunc logs non-fatal configuration loading warnings using structured key/value attributes.
// The signature is compatible with the pattern used by [log/slog]:
//
//	cfg.WarningLogger = func(msg string, attrs ...slog.Attr) {
//	    logger.LogAttrs(context.Background(), slog.LevelWarn, msg, attrs...)
//	}
//
// Callers should keep msg constant and pass dynamic data via attrs
// so that log aggregators can group warnings by their message template.
type WarningFunc func(msg string, attrs ...slog.Attr)

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

// WithProfile sets the configuration profile to load.
// Searches for config/profiles/{profile}.env in standard paths, or only in [WithProfileDir]. A named profile must
// exist. If profile is empty, the ENVIRONMENT env var names an optional profile. Names must match
// ^[a-z0-9][a-z0-9_-]*$.
func WithProfile(profile string) LoaderOption {
	return func(lc *LoaderConfig) {
		lc.Profile = profile
		lc.ProfileEnabled = true
	}
}

// WithProfileDir makes dir the only location searched for the profile's .env file.
func WithProfileDir(dir string) LoaderOption {
	return func(lc *LoaderConfig) { lc.ProfileDir = dir }
}

// WithoutDiscovery loads only explicit files and profiles, skipping the working-directory-relative search.
func WithoutDiscovery() LoaderOption {
	return func(lc *LoaderConfig) { lc.DisableDiscovery = true }
}

// WithWarningLogger sets a warning logger callback for non-fatal loader issues.
func WithWarningLogger(fn WarningFunc) LoaderOption {
	return func(lc *LoaderConfig) { lc.WarningLogger = fn }
}

// LoadConfig loads configuration for a service into the provided cfg struct.
// It searches for config.yml and .env files in standard locations, binds environment variables,
// and unmarshals the result into cfg.
func LoadConfig(serviceName string, cfg any, opts ...LoaderOption) error {
	var lc LoaderConfig
	for _, opt := range opts {
		opt(&lc)
	}
	if lc.FileSystem == nil {
		lc.FileSystem = &RealFileSystem{}
	}

	resolver := &Resolver{FileSystem: lc.FileSystem}
	files, err := resolver.ResolveFiles(serviceName, lc)
	if err != nil {
		return err
	}

	return loadFromResolvedFiles(cfg, loadPlan{
		serviceName: serviceName,
		files:       files,
		fs:          lc.FileSystem,
		warn:        lc.WarningLogger,
	})
}

// Defaultable is implemented by config structs that have default values.
type Defaultable interface {
	ApplyDefaults()
}

// Validatable is implemented by config structs that support validation.
type Validatable interface {
	Validate() error
}

// loadPlan bundles the resolved inputs for loadFromResolvedFiles.
type loadPlan struct {
	serviceName string
	files       ResolvedFiles
	fs          FileSystem
	warn        WarningFunc
}

// loadFromResolvedFiles loads configuration from specific files.
func loadFromResolvedFiles(cfg any, plan loadPlan) error {
	serviceName, files, fs, warn := plan.serviceName, plan.files, plan.fs, plan.warn
	v := viper.New()

	// 1. Load YAML config first (base configuration)
	if files.ConfigFile != "" {
		if err := readConfigFile(v, files.ConfigFile); err != nil {
			return warnAndWrap(warn, "config: failed to load config file", files.ConfigFile, err)
		}
	}

	// 2. Load profile .env file (environment-specific overrides)
	if files.ProfileEnvFile != "" {
		if err := fs.LoadEnv(files.ProfileEnvFile); err != nil {
			return warnAndWrap(warn, "config: failed to load profile env file", files.ProfileEnvFile, err)
		}
	}

	// 3. Enable automatic environment variable reading
	v.AutomaticEnv()
	autoBindEnvVars(v)

	// 4. Load service .env file
	if files.EnvFile != "" {
		if err := fs.LoadEnv(files.EnvFile); err != nil {
			return warnAndWrap(warn, "config: failed to load .env file", files.EnvFile, err)
		}
		// Re-bind env vars after loading .env to pick up new variables
		autoBindEnvVars(v)
	}

	// 5. Unmarshal into config struct (with duration parsing support)
	if err := v.Unmarshal(cfg, viper.DecodeHook(
		mapstructure.ComposeDecodeHookFunc(
			logging.OutputDecodeHook(),
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			mapstructure.TextUnmarshallerHookFunc(),
		),
	)); err != nil {
		return fmt.Errorf("failed to unmarshal config for service %s: %w", serviceName, err)
	}

	// 6. Populate service-name into the config if not already set from the file.
	type serviceConfigAccessor interface {
		GetServiceConfig() *ServiceConfig
	}
	if sc, ok := cfg.(serviceConfigAccessor); ok {
		svc := sc.GetServiceConfig()
		if svc.Name == "" {
			svc.Name = serviceName
		}
	}

	// 7. Apply defaults if the config struct implements Defaultable.
	if d, ok := cfg.(Defaultable); ok {
		d.ApplyDefaults()
	}

	// 8. Validate if the config struct implements Validatable.
	if v, ok := cfg.(Validatable); ok {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("config validation failed for service %s: %w", serviceName, err)
		}
	}

	return nil
}

// warnAndWrap reports a failed load of a resolved file. A file that vanished after resolution still fails as
// ErrFileNotFound rather than being skipped.
func warnAndWrap(warn WarningFunc, msg, file string, err error) error {
	if errors.Is(err, iofs.ErrNotExist) {
		err = fmt.Errorf("%w: %w", ErrFileNotFound, err)
	}
	if warn != nil {
		warn(msg, slog.String("file", file), slog.String("error", err.Error()))
	}
	return fmt.Errorf("%s %q: %w", msg, file, err)
}

func readConfigFile(v *viper.Viper, path string) error {
	if strings.EqualFold(filepath.Ext(path), ".toml") {
		data, err := gokitfs.ReadFileLimit(path, maxStrictFileBytes)
		if err != nil {
			return err
		}
		values, err := codec.Decode[map[string]any](codec.NewTOMLCodec(), string(data))
		if err != nil {
			return err
		}
		return v.MergeConfigMap(values)
	}

	v.SetConfigFile(path)
	return v.ReadInConfig()
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

// autoBindEnvVars binds environment variables to Viper using BindEnv
// so config file values still take precedence over environment variables when expected.
// Only binds variables that match known config key patterns.
func autoBindEnvVars(v *viper.Viper) {
	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) != 2 {
			continue
		}

		key := pair[0]

		variants := generateEnvKeyVariants(key)
		for _, variant := range variants {
			_ = v.BindEnv(variant, key)
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
