// Package version provides immutable build metadata for gokit applications.
//
// Version information is derived from the module's embedded build information (debug.ReadBuildInfo) — VCS revision, modification state, and build time. Release builds may override the defaults at link time via an unexported seam; there is no runtime-mutable version state:
//
//	go build -ldflags "-X github.com/kbukum/gokit/version.buildVersion=1.0.0 \
//	  -X github.com/kbukum/gokit/version.buildGitCommit=$(git rev-parse --short HEAD)"
//
// Use GetVersionInfo for the full VersionInfo, or GetShortVersion / GetFullVersion for formatted strings.
package version
