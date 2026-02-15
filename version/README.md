# version

Build-time version information with Git metadata, injected via `-ldflags`.

## Install

```bash
go get github.com/skillsenselab/gokit
```

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/skillsenselab/gokit/version"
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
go build -ldflags "-X github.com/skillsenselab/gokit/version.Version=1.2.3 \
  -X github.com/skillsenselab/gokit/version.GitCommit=$(git rev-parse --short HEAD) \
  -X github.com/skillsenselab/gokit/version.GitBranch=$(git rev-parse --abbrev-ref HEAD) \
  -X github.com/skillsenselab/gokit/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Info` | Struct with Version, GitCommit, GitBranch, BuildTime, GoVersion |
| `GetVersionInfo()` | Returns full `*Info` struct |
| `GetShortVersion()` | Returns `version-commit` string |
| `GetFullVersion()` | Returns detailed version string |
| `Version` / `GitCommit` / `GitBranch` / `BuildTime` | Linker-injected variables |

---

[⬅ Back to main README](../README.md)
