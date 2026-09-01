package version

import (
	"runtime/debug"
	"strings"
	"time"
)

// Build metadata seam. These are unexported
// and may be overridden at link time via -ldflags "-X github.com/kbukum/gokit/version.buildVersion=...";
// consumers cannot mutate version state at runtime. When left empty,
// values are derived from the module's embedded build information (debug.ReadBuildInfo).
var (
	buildVersion   = "dev"
	buildGitCommit = ""
	buildGitBranch = ""
	buildTime      = ""
	buildGoVersion = ""
)

// VersionInfo is immutable build metadata describing the running binary.
type VersionInfo struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	GitBranch string `json:"git_branch"`
	// BuildTime is the UTC build timestamp in RFC 3339 format, or empty if unavailable.
	BuildTime string `json:"build_time"`
	// BuildDate is the UTC build date (YYYY-MM-DD), omitted when unavailable.
	BuildDate string `json:"build_date,omitempty"`
	GoVersion string `json:"go_version"`
	IsRelease bool   `json:"is_release"`
	IsDirty   bool   `json:"is_dirty"`
}

// source is the raw, injectable input from which a VersionInfo is computed.
// It isolates the pure derivation logic from process-global build state
// so the behavior can be tested deterministically.
type source struct {
	version   string
	gitCommit string
	gitBranch string
	buildTime string
	goVersion string
	buildInfo *debug.BuildInfo
}

// GetVersionInfo returns immutable version information for the running binary,
// derived from link-time overrides when present
// and otherwise from the embedded build information (VCS revision, modification state, and build time).
func GetVersionInfo() *VersionInfo {
	return compute(readSource())
}

// readSource collects the link-time seam and embedded build information.
func readSource() source {
	s := source{
		version:   buildVersion,
		gitCommit: buildGitCommit,
		gitBranch: buildGitBranch,
		buildTime: buildTime,
		goVersion: buildGoVersion,
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		s.buildInfo = info
	}
	return s
}

// compute derives an immutable VersionInfo from the given source. It is pure:
// identical input always yields identical output.
func compute(s source) *VersionInfo {
	info := &VersionInfo{
		Version:   s.version,
		GitCommit: s.gitCommit,
		GitBranch: s.gitBranch,
		GoVersion: s.goVersion,
	}

	info.BuildTime, info.BuildDate = buildTimestamp(s.buildTime)

	if s.buildInfo != nil {
		applyBuildInfo(info, s.buildInfo)
	}

	info.IsRelease = isRelease(s.version, info.IsDirty)

	return info
}

// applyBuildInfo fills unset fields from embedded VCS build information.
func applyBuildInfo(info *VersionInfo, bi *debug.BuildInfo) {
	if info.GoVersion == "" {
		info.GoVersion = bi.GoVersion
	}
	for _, setting := range bi.Settings {
		switch setting.Key {
		case "vcs.revision":
			if info.GitCommit == "" {
				info.GitCommit = shortCommit(setting.Value)
			}
		case "vcs.modified":
			info.IsDirty = setting.Value == "true"
		case "vcs.time":
			if info.BuildTime == "" {
				info.BuildTime, info.BuildDate = buildTimestamp(setting.Value)
			}
		}
	}
}

// buildTimestamp normalizes an RFC 3339 build timestamp to UTC, returning the
// canonical UTC RFC 3339 timestamp and its UTC calendar date (YYYY-MM-DD). Both
// are "" when the timestamp is empty or malformed, so callers never surface an
// unvalidated build time.
func buildTimestamp(buildTime string) (timestamp, date string) {
	t, err := time.Parse(time.RFC3339, buildTime)
	if err != nil {
		return "", ""
	}
	utc := t.UTC()
	return utc.Format(time.RFC3339), utc.Format("2006-01-02")
}

// isRelease reports whether a version string denotes a release build. A dirty
// working tree is never a release, even when the version string looks clean.
func isRelease(version string, dirty bool) bool {
	return version != "dev" && !strings.Contains(version, "dirty") && !dirty
}

// shortCommit truncates a full VCS revision to a 7-character short hash.
func shortCommit(rev string) string {
	if len(rev) > 7 {
		return rev[:7]
	}
	return rev
}
