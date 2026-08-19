package fs

import (
	"fmt"
	"os"
	"path/filepath"

	apperrors "github.com/kbukum/gokit/errors"
)

const maxAppNameLen = 64

// AppCacheDir returns the platform cache directory for appName. The directory is not
// created; callers own creation and cleanup policy.
//
// Platform behavior:
//   - macOS: $HOME/Library/Caches/<appName>
//   - Windows: %LOCALAPPDATA%\<appName>\Cache
//   - Unix: $XDG_CACHE_HOME/<appName> or $HOME/.cache/<appName>
//
// appName must be 1-64 ASCII letters, digits, '-', or '_', so it is always a safe
// single path segment.
func AppCacheDir(appName string) (string, error) {
	return appCacheDirFromEnv(appName, os.LookupEnv)
}

func appCacheDirFromEnv(appName string, env func(string) (string, bool)) (string, error) {
	if err := validateAppName(appName); err != nil {
		return "", err
	}
	base, err := platformCacheBase(env)
	if err != nil {
		return "", err
	}
	return joinAppCacheDir(base, appName), nil
}

func validateAppName(appName string) error {
	if appName == "" || len(appName) > maxAppNameLen {
		return invalidAppNameError()
	}
	for i := 0; i < len(appName); i++ {
		c := appName[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}
		return invalidAppNameError()
	}
	return nil
}

func invalidAppNameError() error {
	return apperrors.InvalidInput("app_name",
		"app name must be 1-64 ASCII letters, digits, '-' or '_'")
}

func envAbsolutePath(key string, env func(string) (string, bool)) (string, error) {
	value, ok := env(key)
	if !ok {
		return "", apperrors.New(apperrors.ErrCodeInvalidInput,
			fmt.Sprintf("%s is required to resolve the application cache directory", key), 422)
	}
	if value == "" {
		return "", apperrors.New(apperrors.ErrCodeInvalidInput,
			fmt.Sprintf("%s cannot be empty", key), 422)
	}
	if !filepath.IsAbs(value) {
		return "", apperrors.New(apperrors.ErrCodeInvalidInput,
			fmt.Sprintf("%s must be an absolute path", key), 422)
	}
	return value, nil
}

// envPathIfAbsolute reads an optional absolute-path env var. Used by the unix and
// windows platform files; the darwin path does not consult it.
//
//nolint:unused // referenced only from platform-specific build variants (unix, windows)
func envPathIfAbsolute(key string, env func(string) (string, bool)) (value string, ok bool, err error) {
	value, ok = env(key)
	if !ok || value == "" {
		return "", false, nil
	}
	if !filepath.IsAbs(value) {
		return "", false, apperrors.New(apperrors.ErrCodeInvalidInput,
			fmt.Sprintf("%s must be an absolute path when set", key), 422)
	}
	return value, true, nil
}
