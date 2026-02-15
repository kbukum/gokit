package version

import (
	"strings"
	"testing"
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
