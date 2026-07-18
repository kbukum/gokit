package version

import (
	"fmt"
	"strings"
)

// GetShortVersion returns a compact version string of the form "<version>[-<commit>][-dirty]".
func GetShortVersion() string {
	return GetVersionInfo().Short()
}

// GetFullVersion returns a detailed version string including branch and build date when available.
func GetFullVersion() string {
	return GetVersionInfo().Full()
}

// Short returns a compact version string of the form "<version>[-<commit>][-dirty]".
func (v *VersionInfo) Short() string {
	if v.GitCommit == "" {
		return v.Version
	}
	if v.IsDirty {
		return fmt.Sprintf("%s-%s-dirty", v.Version, v.GitCommit)
	}
	return fmt.Sprintf("%s-%s", v.Version, v.GitCommit)
}

// Full returns a detailed version string including a non-default branch, dirty state,
// and build date when available.
func (v *VersionInfo) Full() string {
	parts := []string{v.Version}

	if v.GitCommit != "" {
		parts = append(parts, v.GitCommit)
	}
	if v.GitBranch != "" && v.GitBranch != "main" && v.GitBranch != "master" {
		parts = append(parts, v.GitBranch)
	}
	if v.IsDirty {
		parts = append(parts, "dirty")
	}

	full := strings.Join(parts, "-")
	if !v.BuildDate.IsZero() {
		full += fmt.Sprintf(" (built %s)", v.BuildDate.Format("2006-01-02T15:04:05Z"))
	}
	return full
}
