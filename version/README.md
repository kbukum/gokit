# version

Build-time version information with Git metadata,
derived from embedded build info (debug.ReadBuildInfo),
optionally overridden at link time via an unexported seam.

## Install

```bash
go get github.com/kbukum/gokit
```

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/kbukum/gokit/version"
)

func main() {
    info := version.GetVersionInfo()
    fmt.Println(info.Version)   // e.g. "1.2.3"
    fmt.Println(info.GitCommit) // e.g. "abc1234"
    fmt.Println(info.GitBranch) // e.g. "main"

    fmt.Println(version.GetShortVersion()) // "1.2.3-abc1234"
    fmt.Println(version.GetFullVersion())  // "1.2.3-abc1234 (main, 2024-01-15)"
}
```

Build with ldflags:

```bash
go build -ldflags "-X github.com/kbukum/gokit/version.buildVersion=1.2.3 \
  -X github.com/kbukum/gokit/version.buildGitCommit=$(git rev-parse --short HEAD) \
  -X github.com/kbukum/gokit/version.buildGitBranch=$(git rev-parse --abbrev-ref HEAD) \
  -X github.com/kbukum/gokit/version.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `VersionInfo` | Struct with Version, GitCommit, GitBranch, BuildTime, GoVersion |
| `GetVersionInfo()` | Returns full `*VersionInfo` struct |
| `GetShortVersion()` | Returns `version-commit` string |
| `GetFullVersion()` | Returns detailed version string |
| `buildVersion` / `buildGitCommit` / `buildGitBranch` / `buildTime` | Unexported link-time override seam (no runtime mutation) |

---

[⬅ Back to main README](../README.md)
