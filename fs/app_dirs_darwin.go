//go:build darwin

package fs

import "path/filepath"

func platformCacheBase(env func(string) (string, bool)) (string, error) {
	home, err := envAbsolutePath("HOME", env)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Caches"), nil
}

func joinAppCacheDir(base, appName string) string {
	return filepath.Join(base, appName)
}
