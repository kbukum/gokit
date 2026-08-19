package fs

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestAppCacheDirValidatesName(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"", "has space", "has/slash", "a$b"} {
		if _, err := AppCacheDir(name); err == nil {
			t.Errorf("expected invalid app name %q to error", name)
		}
	}
	long := make([]byte, 65)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := AppCacheDir(string(long)); err == nil {
		t.Error("expected 65-char name to error")
	}
}

func TestAppCacheDirFromEnv(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("unix/darwin layout")
	}
	home := "/home/user"
	env := func(key string) (string, bool) {
		switch key {
		case "HOME":
			return home, true
		case "XDG_CACHE_HOME":
			return "", false
		default:
			return "", false
		}
	}
	got, err := appCacheDirFromEnv("myapp", env)
	if err != nil {
		t.Fatal(err)
	}
	var want string
	if runtime.GOOS == "darwin" {
		want = filepath.Join(home, "Library", "Caches", "myapp")
	} else {
		want = filepath.Join(home, ".cache", "myapp")
	}
	if got != want {
		t.Fatalf("AppCacheDir = %q, want %q", got, want)
	}
}

func TestAppCacheDirRejectsRelativeEnv(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("unix/darwin layout")
	}
	env := func(key string) (string, bool) {
		if key == "XDG_CACHE_HOME" && runtime.GOOS != "darwin" {
			return "relative/path", true
		}
		if key == "HOME" {
			return "relative", true
		}
		return "", false
	}
	if _, err := appCacheDirFromEnv("myapp", env); err == nil {
		t.Fatal("expected relative env path to error")
	}
}
