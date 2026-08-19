//go:build windows

package fs

import "path/filepath"

func platformCacheBase(env func(string) (string, bool)) (string, error) {
	if localAppData, ok, err := envPathIfAbsolute("LOCALAPPDATA", env); err != nil {
		return "", err
	} else if ok {
		return localAppData, nil
	}
	home, err := envAbsolutePath("USERPROFILE", env)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "AppData", "Local"), nil
}

func joinAppCacheDir(base, appName string) string {
	return filepath.Join(base, appName, "Cache")
}
