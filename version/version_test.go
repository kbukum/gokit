package version

import (
	"encoding/json"
	"runtime/debug"
	"strings"
	"testing"
)

// src builds a source with no embedded build info, giving deterministic,
// override-only derivation for tests.
func src(version, commit, branch, buildTime, goVersion string) source {
	return source{
		version:   version,
		gitCommit: commit,
		gitBranch: branch,
		buildTime: buildTime,
		goVersion: goVersion,
	}
}

func TestComputeDefaults(t *testing.T) {
	t.Parallel()
	info := compute(src("dev", "", "", "", ""))
	if info.Version != "dev" {
		t.Errorf("expected version 'dev', got %q", info.Version)
	}
	if info.IsRelease {
		t.Error("dev should not be a release")
	}
	if info.BuildDate != "" {
		t.Errorf("expected empty BuildDate for dev build, got %q", info.BuildDate)
	}
}

func TestComputeWithBuildTime(t *testing.T) {
	t.Parallel()
	info := compute(src("1.0.0", "abc1234", "main", "2024-01-15T10:30:00Z", "go1.22.0"))
	if info.Version != "1.0.0" {
		t.Errorf("expected '1.0.0', got %q", info.Version)
	}
	if !info.IsRelease {
		t.Error("1.0.0 should be a release")
	}
	if info.GitCommit != "abc1234" {
		t.Errorf("expected 'abc1234', got %q", info.GitCommit)
	}
	if info.GoVersion != "go1.22.0" {
		t.Errorf("expected 'go1.22.0', got %q", info.GoVersion)
	}
	if info.BuildDate != "2024-01-15" {
		t.Errorf("expected build date 2024-01-15, got %q", info.BuildDate)
	}
}

func TestComputeInvalidBuildTime(t *testing.T) {
	t.Parallel()
	info := compute(src("1.0.0", "", "", "not-a-date", ""))
	if info.Version != "1.0.0" {
		t.Errorf("expected '1.0.0', got %q", info.Version)
	}
	if info.BuildDate != "" {
		t.Errorf("invalid build time must leave BuildDate empty, got %q", info.BuildDate)
	}
}

func TestIsReleaseTable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		version   string
		isRelease bool
	}{
		{"1.0.0", true},
		{"dev", false},
		{"1.0.0-dirty", false},
		{"v2.1.0", true},
		{"0.0.0", true},
		{"dirty", false},
		{"1.0.0-rc1", true},
		{"1.0.0-alpha.1", true},
		{"1.0.0-beta.2", true},
		{"DEV", true},                // case-sensitive: only lowercase "dev" is non-release
		{"1.0.0-dirty-build", false}, // contains "dirty"
		{"", true},                   // empty is not "dev" and has no "dirty"
	}
	for _, tc := range tests {
		t.Run(tc.version, func(t *testing.T) {
			t.Parallel()
			info := compute(src(tc.version, "", "", "", ""))
			if info.IsRelease != tc.isRelease {
				t.Errorf("Version=%q: expected IsRelease=%v, got %v", tc.version, tc.isRelease, info.IsRelease)
			}
		})
	}
}

func TestApplyBuildInfoFillsUnsetFields(t *testing.T) {
	t.Parallel()
	bi := &debug.BuildInfo{
		GoVersion: "go1.25.0",
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef1234567890"},
			{Key: "vcs.modified", Value: "true"},
			{Key: "vcs.time", Value: "2025-02-03T04:05:06Z"},
		},
	}
	info := compute(source{version: "dev", buildInfo: bi})
	if info.GoVersion != "go1.25.0" {
		t.Errorf("expected GoVersion from build info, got %q", info.GoVersion)
	}
	if info.GitCommit != "abcdef1" {
		t.Errorf("expected VCS revision truncated to 7 chars, got %q", info.GitCommit)
	}
	if !info.IsDirty {
		t.Error("expected IsDirty true from vcs.modified")
	}
	if info.BuildDate != "2025-02-03" {
		t.Errorf("expected BuildDate from vcs.time, got %q", info.BuildDate)
	}
	if info.BuildTime != "2025-02-03T04:05:06Z" {
		t.Errorf("expected BuildTime from vcs.time, got %q", info.BuildTime)
	}
}

func TestApplyBuildInfoDoesNotOverrideExplicit(t *testing.T) {
	t.Parallel()
	bi := &debug.BuildInfo{
		GoVersion: "go1.25.0",
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef1234567890"},
			{Key: "vcs.time", Value: "2025-02-03T04:05:06Z"},
		},
	}
	// Explicit values from the ldflags seam take precedence over VCS info.
	info := compute(source{
		version:   "1.0.0",
		gitCommit: "explicit",
		buildTime: "2020-01-01T00:00:00Z",
		goVersion: "go1.20.0",
		buildInfo: bi,
	})
	if info.GitCommit != "explicit" {
		t.Errorf("explicit commit must win, got %q", info.GitCommit)
	}
	if info.GoVersion != "go1.20.0" {
		t.Errorf("explicit go version must win, got %q", info.GoVersion)
	}
	if info.BuildTime != "2020-01-01T00:00:00Z" {
		t.Errorf("explicit build time must win, got %q", info.BuildTime)
	}
}

func TestShortDev(t *testing.T) {
	t.Parallel()
	sv := compute(src("dev", "", "", "", "")).Short()
	if sv != "dev" {
		t.Errorf("expected 'dev', got %q", sv)
	}
}

func TestShortWithCommit(t *testing.T) {
	t.Parallel()
	sv := compute(src("1.0.0", "abc1234", "", "2024-01-01T00:00:00Z", "go1.22")).Short()
	if sv != "1.0.0-abc1234" {
		t.Errorf("expected '1.0.0-abc1234', got %q", sv)
	}
}

func TestShortDirty(t *testing.T) {
	t.Parallel()
	info := compute(src("1.0.0", "abc1234", "", "", ""))
	info.IsDirty = true
	if got := info.Short(); got != "1.0.0-abc1234-dirty" {
		t.Errorf("expected dirty short version, got %q", got)
	}
}

func TestShortNoCommit(t *testing.T) {
	t.Parallel()
	if got := compute(src("1.5.0", "", "", "", "")).Short(); got != "1.5.0" {
		t.Errorf("expected '1.5.0', got %q", got)
	}
}

func TestFullBasic(t *testing.T) {
	t.Parallel()
	fv := compute(src("1.0.0", "abc1234", "main", "2024-01-15T10:30:00Z", "go1.22")).Full()
	if !strings.Contains(fv, "1.0.0") || !strings.Contains(fv, "abc1234") {
		t.Errorf("expected version and commit, got %q", fv)
	}
	if strings.Contains(fv, "main") {
		t.Errorf("main branch should not appear, got %q", fv)
	}
	if !strings.Contains(fv, "built") {
		t.Errorf("expected 'built', got %q", fv)
	}
	if !strings.Contains(fv, "2024-01-15T10:30:00Z") {
		t.Errorf("expected ISO build date, got %q", fv)
	}
}

func TestFullFeatureBranch(t *testing.T) {
	t.Parallel()
	fv := compute(src("1.0.0", "abc1234", "feature/new-thing", "2024-01-15T10:30:00Z", "go1.22")).Full()
	if !strings.Contains(fv, "feature/new-thing") {
		t.Errorf("expected feature branch, got %q", fv)
	}
}

func TestFullMasterBranchHidden(t *testing.T) {
	t.Parallel()
	fv := compute(src("1.0.0", "abc1234", "master", "2024-01-15T10:30:00Z", "go1.22")).Full()
	if strings.Contains(fv, "master") {
		t.Errorf("master branch should not appear, got %q", fv)
	}
}

func TestFullNoCommit(t *testing.T) {
	t.Parallel()
	fv := compute(src("dev", "", "", "", "")).Full()
	if fv != "dev" {
		t.Errorf("expected 'dev', got %q", fv)
	}
}

func TestFullDirty(t *testing.T) {
	t.Parallel()
	info := compute(src("1.0.0", "abc1234", "feature/test", "2024-01-15T10:30:00Z", "go1.22"))
	info.IsDirty = true
	fv := info.Full()
	if !strings.Contains(fv, "dirty") {
		t.Errorf("expected 'dirty' in full version, got %q", fv)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	t.Parallel()
	original := compute(src("4.0.0", "eee5555", "feature/json-test", "2025-01-01T00:00:00Z", "go1.23"))

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var restored VersionInfo
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if restored != *original {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", restored, *original)
	}
}

// Integration: package-level accessors must not panic and must reflect the
// build-info-derived state of the test binary.
func TestPackageAccessors(t *testing.T) {
	t.Parallel()
	info := GetVersionInfo()
	if info == nil {
		t.Fatal("GetVersionInfo returned nil")
	}
	if GetShortVersion() == "" {
		t.Error("GetShortVersion returned empty string")
	}
	if GetFullVersion() == "" {
		t.Error("GetFullVersion returned empty string")
	}
}

func TestBuildDateFarFuture(t *testing.T) {
	t.Parallel()
	info := compute(src("1.0.0", "ddd4444", "", "2099-12-31T23:59:59Z", "go1.22"))
	if info.BuildDate != "2099-12-31" {
		t.Errorf("expected BuildDate 2099-12-31, got %q", info.BuildDate)
	}
}

// A dirty working tree is never a release, even when the version string is clean.
func TestIsReleaseDirtyTreeIsNotRelease(t *testing.T) {
	t.Parallel()
	bi := &debug.BuildInfo{
		Settings: []debug.BuildSetting{
			{Key: "vcs.modified", Value: "true"},
		},
	}
	info := compute(source{version: "1.0.0", buildInfo: bi})
	if !info.IsDirty {
		t.Fatal("expected IsDirty true from vcs.modified")
	}
	if info.IsRelease {
		t.Error("dirty build of a clean version must not be a release")
	}
}
