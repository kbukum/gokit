//go:build unix && !darwin

package fs

import "path/filepath"

func platformCacheBase(env func(string) (string, bool)) (string, error) {
	if xdgCacheHome, ok, err := envPathIfAbsolute("XDG_CACHE_HOME", env); err != nil {
		return "", err
	} else if ok {
		return xdgCacheHome, nil
	}
	home, err := envAbsolutePath("HOME", env)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache"), nil
}

func joinAppCacheDir(base, appName string) string {
	return filepath.Join(base, appName)
}
