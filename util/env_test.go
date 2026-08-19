package util

import (
	"strconv"
	"testing"
)

func TestGetEnv(t *testing.T) {
	t.Setenv("GOKIT_UTIL_TEST_ENV", "value")
	if v, ok := GetEnv("GOKIT_UTIL_TEST_ENV"); !ok || v != "value" {
		t.Fatalf("GetEnv = (%q, %v)", v, ok)
	}
	if _, ok := GetEnv("GOKIT_UTIL_TEST_MISSING_XYZ"); ok {
		t.Fatal("expected missing var to report false")
	}
}

func TestGetEnvNonEmpty(t *testing.T) {
	t.Setenv("GOKIT_UTIL_TEST_EMPTY", "")
	if _, ok := GetEnvNonEmpty("GOKIT_UTIL_TEST_EMPTY"); ok {
		t.Fatal("empty value should report false")
	}
}

func TestGetEnvOr(t *testing.T) {
	if got := GetEnvOr("GOKIT_UTIL_TEST_MISSING_XYZ", "fallback"); got != "fallback" {
		t.Fatalf("GetEnvOr = %q", got)
	}
}

func TestGetEnvParsed(t *testing.T) {
	t.Setenv("GOKIT_UTIL_TEST_PORT", "8080")
	port, ok := GetEnvParsed("GOKIT_UTIL_TEST_PORT", strconv.Atoi)
	if !ok || port != 8080 {
		t.Fatalf("GetEnvParsed = (%d, %v)", port, ok)
	}
	t.Setenv("GOKIT_UTIL_TEST_PORT", "notnum")
	if _, ok := GetEnvParsed("GOKIT_UTIL_TEST_PORT", strconv.Atoi); ok {
		t.Fatal("bad parse should report false")
	}
}

func TestGetEnvBool(t *testing.T) {
	for _, truthy := range []string{"true", "1", "yes", "on", "TRUE", " On "} {
		t.Setenv("GOKIT_UTIL_TEST_BOOL", truthy)
		if !GetEnvBool("GOKIT_UTIL_TEST_BOOL", false) {
			t.Errorf("%q should be true", truthy)
		}
	}
	for _, falsy := range []string{"false", "0", "no", "off"} {
		t.Setenv("GOKIT_UTIL_TEST_BOOL", falsy)
		if GetEnvBool("GOKIT_UTIL_TEST_BOOL", true) {
			t.Errorf("%q should be false", falsy)
		}
	}
	t.Setenv("GOKIT_UTIL_TEST_BOOL", "maybe")
	if !GetEnvBool("GOKIT_UTIL_TEST_BOOL", true) {
		t.Error("unrecognized should fall back")
	}
}
