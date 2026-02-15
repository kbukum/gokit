// Package version provides build version information embedding.
package version

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"
)

var (
	// These variables are set at build time using -ldflags
	Version   = "dev"
	GitCommit = ""
	GitBranch = ""
	BuildTime = ""
	GoVersion = ""
)

// Info represents version information.
type Info struct {
	Version   string    `json:"version"`
	GitCommit string    `json:"git_commit"`
	GitBranch string    `json:"git_branch"`
	BuildTime string    `json:"build_time"`
	GoVersion string    `json:"go_version"`
	BuildDate time.Time `json:"build_date"`
	IsRelease bool      `json:"is_release"`
	IsDirty   bool      `json:"is_dirty"`
}

// GetVersionInfo returns comprehensive version information.
func GetVersionInfo() *Info {
	info := &Info{
		Version:   Version,
		GitCommit: GitCommit,
		GitBranch: GitBranch,
		BuildTime: BuildTime,
		GoVersion: GoVersion,
		IsRelease: Version != "dev" && !strings.Contains(Version, "dirty"),
	}

	if BuildTime != "" {
		if t, err := time.Parse(time.RFC3339, BuildTime); err == nil {
			info.BuildDate = t
		}
	}

	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		if GoVersion == "" {
			info.GoVersion = buildInfo.GoVersion
		}
		for _, setting := range buildInfo.Settings {
			switch setting.Key {
			case "vcs.revision":
				if GitCommit == "" {
					info.GitCommit = setting.Value
					if len(info.GitCommit) > 7 {
						info.GitCommit = info.GitCommit[:7]
					}
				}
			case "vcs.modified":
				info.IsDirty = setting.Value == "true"
			case "vcs.time":
				if BuildTime == "" {
					if t, err := time.Parse(time.RFC3339, setting.Value); err == nil {
						info.BuildDate = t
						info.BuildTime = setting.Value
					}
				}
			}
		}
	}

	if info.BuildDate.IsZero() {
		info.BuildDate = time.Now().UTC()
		info.BuildTime = info.BuildDate.Format(time.RFC3339)
	}

	return info
}

// GetShortVersion returns a short version string.
func GetShortVersion() string {
	info := GetVersionInfo()
	if info.GitCommit != "" {
		if info.IsDirty {
			return fmt.Sprintf("%s-%s-dirty", info.Version, info.GitCommit)
		}
		return fmt.Sprintf("%s-%s", info.Version, info.GitCommit)
	}
	return info.Version
}

// GetFullVersion returns a detailed version string.
func GetFullVersion() string {
	info := GetVersionInfo()
	parts := []string{info.Version}
	if info.GitCommit != "" {
		parts = append(parts, info.GitCommit)
	}
	if info.GitBranch != "" && info.GitBranch != "main" && info.GitBranch != "master" {
		parts = append(parts, info.GitBranch)
	}
	if info.IsDirty {
		parts = append(parts, "dirty")
	}
	version := strings.Join(parts, "-")
	if !info.BuildDate.IsZero() {
		version += fmt.Sprintf(" (built %s)", info.BuildDate.Format("2006-01-02T15:04:05Z"))
	}
	return version
}
