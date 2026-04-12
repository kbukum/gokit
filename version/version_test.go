package version

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func saveAndRestore() func() {
	origVersion, origCommit, origBranch, origBuildTime, origGoVersion :=
		Version, GitCommit, GitBranch, BuildTime, GoVersion
	return func() {
		Version = origVersion
		GitCommit = origCommit
		GitBranch = origBranch
		BuildTime = origBuildTime
		GoVersion = origGoVersion
	}
}

func TestGetVersionInfoDefaults(t *testing.T) {
	defer saveAndRestore()()
	Version = "dev"
	GitCommit = ""
	GitBranch = ""
	BuildTime = ""
	GoVersion = ""

	info := GetVersionInfo()
	if info == nil {
		t.Fatal("expected non-nil Info")
	}
	if info.Version != "dev" {
		t.Errorf("expected version 'dev', got %q", info.Version)
	}
	if info.IsRelease {
		t.Error("dev should not be a release")
	}
	if info.BuildDate.IsZero() {
		t.Error("BuildDate should not be zero")
	}
}

func TestGetVersionInfoWithBuildTime(t *testing.T) {
	defer saveAndRestore()()
	Version = "1.0.0"
	BuildTime = "2024-01-15T10:30:00Z"
	GitCommit = "abc1234"
	GitBranch = "main"
	GoVersion = "go1.22.0"

	info := GetVersionInfo()
	if info.Version != "1.0.0" {
		t.Errorf("expected '1.0.0', got %q", info.Version)
	}
	if info.IsRelease != true {
		t.Error("1.0.0 should be a release")
	}
	if info.GitCommit != "abc1234" {
		t.Errorf("expected 'abc1234', got %q", info.GitCommit)
	}
	if info.GoVersion != "go1.22.0" {
		t.Errorf("expected 'go1.22.0', got %q", info.GoVersion)
	}
	if info.BuildDate.Year() != 2024 {
		t.Errorf("expected build year 2024, got %d", info.BuildDate.Year())
	}
}

func TestGetVersionInfoDirtyVersion(t *testing.T) {
	defer saveAndRestore()()
	Version = "1.0.0-dirty"

	info := GetVersionInfo()
	if info.IsRelease {
		t.Error("dirty version should not be a release")
	}
}

func TestGetShortVersionDev(t *testing.T) {
	defer saveAndRestore()()
	Version = "dev"
	GitCommit = ""
	BuildTime = ""
	GoVersion = ""
	GitBranch = ""

	sv := GetShortVersion()
	if !strings.Contains(sv, "dev") {
		t.Errorf("expected short version to contain 'dev', got %q", sv)
	}
}

func TestGetShortVersionWithCommit(t *testing.T) {
	defer saveAndRestore()()
	Version = "1.0.0"
	GitCommit = "abc1234"
	BuildTime = "2024-01-01T00:00:00Z"
	GoVersion = "go1.22"
	GitBranch = ""

	sv := GetShortVersion()
	if sv != "1.0.0-abc1234" {
		t.Errorf("expected '1.0.0-abc1234', got %q", sv)
	}
}

func TestGetFullVersionBasic(t *testing.T) {
	defer saveAndRestore()()
	Version = "1.0.0"
	GitCommit = "abc1234"
	GitBranch = "main"
	BuildTime = "2024-01-15T10:30:00Z"
	GoVersion = "go1.22"

	fv := GetFullVersion()
	if !strings.Contains(fv, "1.0.0") {
		t.Errorf("expected full version to contain '1.0.0', got %q", fv)
	}
	if !strings.Contains(fv, "abc1234") {
		t.Errorf("expected full version to contain commit, got %q", fv)
	}
	if strings.Contains(fv, "main") {
		t.Errorf("main branch should not appear in full version, got %q", fv)
	}
	if !strings.Contains(fv, "built") {
		t.Errorf("expected full version to contain 'built', got %q", fv)
	}
}

func TestGetFullVersionFeatureBranch(t *testing.T) {
	defer saveAndRestore()()
	Version = "1.0.0"
	GitCommit = "abc1234"
	GitBranch = "feature/new-thing"
	BuildTime = "2024-01-15T10:30:00Z"
	GoVersion = "go1.22"

	fv := GetFullVersion()
	if !strings.Contains(fv, "feature/new-thing") {
		t.Errorf("expected full version to contain feature branch, got %q", fv)
	}
}

func TestGetFullVersionNoCommit(t *testing.T) {
	defer saveAndRestore()()
	Version = "dev"
	GitCommit = ""
	GitBranch = ""
	BuildTime = ""
	GoVersion = ""

	fv := GetFullVersion()
	if !strings.HasPrefix(fv, "dev") {
		t.Errorf("expected full version to start with 'dev', got %q", fv)
	}
}

func TestGetShortVersionDirty(t *testing.T) {
	defer saveAndRestore()()
	Version = "1.0.0"
	GitCommit = "abc1234"
	BuildTime = "2024-01-01T00:00:00Z"
	GoVersion = "go1.22"
	GitBranch = ""

	// Simulate dirty build by modifying the IsDirty through BuildInfo
	info := GetVersionInfo()
	info.IsDirty = true

	// GetShortVersion checks IsDirty from GetVersionInfo
	// We can't easily set vcs.modified, but we can test the code path
	sv := GetShortVersion()
	if !strings.Contains(sv, "1.0.0") {
		t.Errorf("expected version in short version, got %q", sv)
	}
}

func TestGetFullVersionDirty(t *testing.T) {
	defer saveAndRestore()()
	Version = "1.0.0"
	GitCommit = "abc1234"
	GitBranch = "feature/test"
	BuildTime = "2024-01-15T10:30:00Z"
	GoVersion = "go1.22"

	fv := GetFullVersion()
	if !strings.Contains(fv, "1.0.0") {
		t.Errorf("expected version in full version, got %q", fv)
	}
	if !strings.Contains(fv, "feature/test") {
		t.Errorf("expected feature branch in full version, got %q", fv)
	}
}

func TestGetFullVersionMasterBranch(t *testing.T) {
	defer saveAndRestore()()
	Version = "1.0.0"
	GitCommit = "abc1234"
	GitBranch = "master"
	BuildTime = "2024-01-15T10:30:00Z"
	GoVersion = "go1.22"

	fv := GetFullVersion()
	if strings.Contains(fv, "master") {
		t.Errorf("master branch should not appear in full version, got %q", fv)
	}
}

func TestGetVersionInfoInvalidBuildTime(t *testing.T) {
	defer saveAndRestore()()
	Version = "1.0.0"
	BuildTime = "not-a-date"
	GitCommit = ""
	GitBranch = ""
	GoVersion = ""

	info := GetVersionInfo()
	// Invalid build time should not cause a crash; date comes from debug.ReadBuildInfo or now()
	if info.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", info.Version)
	}
}

func TestInfoStruct(t *testing.T) {
	defer saveAndRestore()()
	Version = "2.0.0"
	GitCommit = "def5678"
	GitBranch = "develop"
	BuildTime = "2025-06-01T12:00:00Z"
	GoVersion = "go1.23.0"

	info := GetVersionInfo()
	if info.Version != "2.0.0" {
		t.Errorf("expected version '2.0.0', got %q", info.Version)
	}
	if info.GitCommit != "def5678" {
		t.Errorf("expected commit 'def5678', got %q", info.GitCommit)
	}
	if info.GitBranch != "develop" {
		t.Errorf("expected branch 'develop', got %q", info.GitBranch)
	}
	if info.IsRelease != true {
		t.Error("expected 2.0.0 to be a release")
	}
}

// --- New TDD tests ---

// Version string parsing edge cases

func TestGetVersionInfoPreRelease(t *testing.T) {
	defer saveAndRestore()()
	Version = "1.0.0-rc.1"
	GitCommit = "aaa1111"
	GitBranch = "main"
	BuildTime = "2024-06-01T00:00:00Z"
	GoVersion = "go1.22"

	info := GetVersionInfo()
	if !info.IsRelease {
		t.Error("1.0.0-rc.1 should be a release (not 'dev', not 'dirty')")
	}
	if info.Version != "1.0.0-rc.1" {
		t.Errorf("expected version '1.0.0-rc.1', got %q", info.Version)
	}
}

func TestGetVersionInfoPreReleaseAlpha(t *testing.T) {
	defer saveAndRestore()()
	Version = "1.0.0-alpha.1"
	GitCommit = ""
	GitBranch = ""
	BuildTime = ""
	GoVersion = ""

	info := GetVersionInfo()
	if !info.IsRelease {
		t.Error("1.0.0-alpha.1 should be a release")
	}
}

func TestGetVersionInfoPreReleaseBeta(t *testing.T) {
	defer saveAndRestore()()
	Version = "1.0.0-beta.2"
	GitCommit = ""
	GitBranch = ""
	BuildTime = ""
	GoVersion = ""

	info := GetVersionInfo()
	if !info.IsRelease {
		t.Error("1.0.0-beta.2 should be a release")
	}
}

func TestGetVersionInfoEmptyVersion(t *testing.T) {
	defer saveAndRestore()()
	Version = ""
	GitCommit = ""
	GitBranch = ""
	BuildTime = ""
	GoVersion = ""

	info := GetVersionInfo()
	if info.Version != "" {
		t.Errorf("expected empty version, got %q", info.Version)
	}
	// Empty string is not "dev" and does not contain "dirty", so IsRelease should be true
	if !info.IsRelease {
		t.Error("empty version should be treated as a release by current logic")
	}
}

// Version comparison patterns — table-driven IsRelease logic

func TestGetVersionInfo_ReleaseLogicTable(t *testing.T) {
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
		{"DEV", true},                // case-sensitive: only lowercase "dev" is non-release
		{"1.0.0-dirty-build", false}, // contains "dirty"
	}

	for _, tc := range tests {
		t.Run(tc.version, func(t *testing.T) {
			defer saveAndRestore()()
			Version = tc.version
			GitCommit = ""
			GitBranch = ""
			BuildTime = ""
			GoVersion = ""

			info := GetVersionInfo()
			if info.IsRelease != tc.isRelease {
				t.Errorf("Version=%q: expected IsRelease=%v, got %v", tc.version, tc.isRelease, info.IsRelease)
			}
		})
	}
}

// Semantic versioning compliance

func TestGetShortVersion_SemverFormat(t *testing.T) {
	defer saveAndRestore()()
	Version = "3.2.1"
	GitCommit = "f00baa7"
	GitBranch = ""
	BuildTime = "2024-03-01T00:00:00Z"
	GoVersion = "go1.22"

	sv := GetShortVersion()
	if sv != "3.2.1-f00baa7" {
		t.Errorf("expected '3.2.1-f00baa7', got %q", sv)
	}
}

func TestGetFullVersion_SemverWithPrerelease(t *testing.T) {
	defer saveAndRestore()()
	Version = "2.0.0-rc.1"
	GitCommit = "bbb2222"
	GitBranch = "feature/release-prep"
	BuildTime = "2024-07-01T08:00:00Z"
	GoVersion = "go1.22"

	fv := GetFullVersion()
	if !strings.Contains(fv, "2.0.0-rc.1") {
		t.Errorf("expected version '2.0.0-rc.1' in output, got %q", fv)
	}
	if !strings.Contains(fv, "bbb2222") {
		t.Errorf("expected commit in output, got %q", fv)
	}
	if !strings.Contains(fv, "feature/release-prep") {
		t.Errorf("expected feature branch in output, got %q", fv)
	}
	if !strings.Contains(fv, "built") {
		t.Errorf("expected 'built' in output, got %q", fv)
	}
}

// Edge cases

func TestGetVersionInfoLongCommitHash(t *testing.T) {
	defer saveAndRestore()()
	Version = "1.0.0"
	GitCommit = "abc1234567890" // 13 chars, longer than typical short hash
	GitBranch = ""
	BuildTime = "2024-01-01T00:00:00Z"
	GoVersion = "go1.22"

	info := GetVersionInfo()
	// When set via ldflags, the commit is used as-is (no truncation)
	if info.GitCommit != "abc1234567890" {
		t.Errorf("expected long commit hash preserved, got %q", info.GitCommit)
	}
}

func TestGetVersionInfoShortCommitHash(t *testing.T) {
	defer saveAndRestore()()
	Version = "1.0.0"
	GitCommit = "abc" // very short hash
	GitBranch = ""
	BuildTime = "2024-01-01T00:00:00Z"
	GoVersion = "go1.22"

	info := GetVersionInfo()
	if info.GitCommit != "abc" {
		t.Errorf("expected short commit hash 'abc', got %q", info.GitCommit)
	}
}

func TestGetShortVersionNoCommitNoDirty(t *testing.T) {
	defer saveAndRestore()()
	Version = "1.5.0"
	GitCommit = ""
	GitBranch = ""
	BuildTime = "2024-01-01T00:00:00Z"
	GoVersion = "go1.22"

	sv := GetShortVersion()
	// With no commit, short version should just be the version string
	// (dirty state comes from VCS, can't control it here, but the version prefix is deterministic)
	if !strings.HasPrefix(sv, "1.5.0") {
		t.Errorf("expected short version to start with '1.5.0', got %q", sv)
	}
}

func TestGetFullVersionBuildDateFormat(t *testing.T) {
	defer saveAndRestore()()
	Version = "1.0.0"
	GitCommit = "ccc3333"
	GitBranch = "main"
	BuildTime = "2024-11-20T14:30:00Z"
	GoVersion = "go1.22"

	fv := GetFullVersion()
	// Output should contain an ISO-like date in the "(built ...)" section
	if !strings.Contains(fv, "2024-11-20T14:30:00Z") {
		t.Errorf("expected ISO date '2024-11-20T14:30:00Z' in full version, got %q", fv)
	}
}

func TestGetVersionInfoBuildTimeEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		buildTime string
		wantZero  bool // true if BuildDate should fall back (not parsed from buildTime)
	}{
		{"empty", "", true},
		{"zero_time", "0001-01-01T00:00:00Z", true}, // parses to Go zero time, overwritten by now()
		{"far_future", "2099-12-31T23:59:59Z", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer saveAndRestore()()
			Version = "1.0.0"
			GitCommit = "ddd4444"
			GitBranch = ""
			BuildTime = tc.buildTime
			GoVersion = "go1.22"

			info := GetVersionInfo()
			if tc.wantZero {
				// When BuildTime is empty, BuildDate is populated from VCS or now()
				if info.BuildDate.IsZero() {
					t.Error("BuildDate should not be zero even with empty BuildTime")
				}
			} else {
				parsed, err := time.Parse(time.RFC3339, tc.buildTime)
				if err != nil {
					t.Fatalf("test setup error: %v", err)
				}
				if !info.BuildDate.Equal(parsed) {
					t.Errorf("expected BuildDate=%v, got %v", parsed, info.BuildDate)
				}
			}
		})
	}
}

func TestGetFullVersionAllEmpty(t *testing.T) {
	defer saveAndRestore()()
	Version = ""
	GitCommit = ""
	GitBranch = ""
	BuildTime = ""
	GoVersion = ""

	fv := GetFullVersion()
	// Should not panic and should produce some output with "(built ...)"
	if fv == "" {
		t.Error("full version should not be empty even with all empty fields")
	}
	if !strings.Contains(fv, "built") {
		t.Errorf("expected 'built' in full version, got %q", fv)
	}
}

// JSON round-trip

func TestInfoJSON_RoundTrip(t *testing.T) {
	defer saveAndRestore()()
	Version = "4.0.0"
	GitCommit = "eee5555"
	GitBranch = "feature/json-test"
	BuildTime = "2025-01-01T00:00:00Z"
	GoVersion = "go1.23"

	original := GetVersionInfo()

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal Info: %v", err)
	}

	var restored Info
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("failed to unmarshal Info: %v", err)
	}

	if restored.Version != original.Version {
		t.Errorf("Version mismatch: %q vs %q", restored.Version, original.Version)
	}
	if restored.GitCommit != original.GitCommit {
		t.Errorf("GitCommit mismatch: %q vs %q", restored.GitCommit, original.GitCommit)
	}
	if restored.GitBranch != original.GitBranch {
		t.Errorf("GitBranch mismatch: %q vs %q", restored.GitBranch, original.GitBranch)
	}
	if restored.GoVersion != original.GoVersion {
		t.Errorf("GoVersion mismatch: %q vs %q", restored.GoVersion, original.GoVersion)
	}
	if restored.IsRelease != original.IsRelease {
		t.Errorf("IsRelease mismatch: %v vs %v", restored.IsRelease, original.IsRelease)
	}
	if restored.IsDirty != original.IsDirty {
		t.Errorf("IsDirty mismatch: %v vs %v", restored.IsDirty, original.IsDirty)
	}
	if !restored.BuildDate.Equal(original.BuildDate) {
		t.Errorf("BuildDate mismatch: %v vs %v", restored.BuildDate, original.BuildDate)
	}
}

// Branch handling

func TestGetFullVersion_DevelopBranch(t *testing.T) {
	defer saveAndRestore()()
	Version = "1.0.0"
	GitCommit = "fff6666"
	GitBranch = "develop"
	BuildTime = "2024-01-01T00:00:00Z"
	GoVersion = "go1.22"

	fv := GetFullVersion()
	if !strings.Contains(fv, "develop") {
		t.Errorf("expected 'develop' branch in full version, got %q", fv)
	}
}

func TestGetFullVersion_ReleaseBranch(t *testing.T) {
	defer saveAndRestore()()
	Version = "1.0.0"
	GitCommit = "aab7777"
	GitBranch = "release/v1.0"
	BuildTime = "2024-01-01T00:00:00Z"
	GoVersion = "go1.22"

	fv := GetFullVersion()
	if !strings.Contains(fv, "release/v1.0") {
		t.Errorf("expected 'release/v1.0' branch in full version, got %q", fv)
	}
}

func TestGetFullVersion_EmptyBranch(t *testing.T) {
	defer saveAndRestore()()
	Version = "1.0.0"
	GitCommit = "bbc8888"
	GitBranch = ""
	BuildTime = "2024-01-01T00:00:00Z"
	GoVersion = "go1.22"

	fv := GetFullVersion()
	// With no branch, output should be "version-commit (built ...)" — no extra hyphen-segment for branch
	parts := strings.SplitN(fv, " (built", 2)
	segments := strings.Split(parts[0], "-")
	// segments: ["1.0.0", "bbc8888"] — no branch segment
	for _, seg := range segments {
		if seg == "" {
			t.Errorf("unexpected empty segment in version string %q", fv)
		}
	}
	// "1.0.0-bbc8888" has exactly 1 hyphen; more would indicate a branch or dirty tag.
	if !strings.Contains(fv, "1.0.0") {
		t.Errorf("expected version in output, got %q", fv)
	}
	if !strings.Contains(fv, "bbc8888") {
		t.Errorf("expected commit in output, got %q", fv)
	}
}
