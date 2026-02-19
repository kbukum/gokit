// Package version provides build version information embedding for
// gokit applications.
//
// Version, git commit, branch, and build time are set at compile time
// via -ldflags:
//
//	go build -ldflags "-X github.com/kbukum/gokit/version.Version=1.0.0"
package version
