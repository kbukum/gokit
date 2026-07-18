# git

Domain-grouped interfaces and backend orchestration for Git repository operations.
The root package exposes capability-oriented interfaces (refs, remotes, config, diff, log, blame, index, commits, …)
and shared types; concrete implementations live in backend sub-packages (`embedded` and `cli`).
A `*Repo` composes those backends behind a single ergonomic surface.

## Install

```bash
go get github.com/kbukum/gokit/git
```

## Quick Start

```go
package main

import (
    "fmt"

    "github.com/kbukum/gokit/git"
)

func main() {
    repo, err := git.Open(".")
    if err != nil {
        panic(err)
    }

    head, err := repo.Head()
    if err != nil {
        panic(err)
    }
    fmt.Println("HEAD:", head)

    dirty, _ := repo.IsDirty()
    fmt.Println("dirty:", dirty)
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Open()` / `Discover()` | Open a repository at, or by discovering upward from, a path |
| `Clone()` / `Init()` / `InitBare()` | Create repositories |
| `Repo` | Composed repository handle over the embedded + CLI backends |
| `Repository` / `Executor` / `RefManager` / `RemoteManager` / `ConfigReader` | Capability interfaces |
| `Differ` / `TreeReader` / `LogReader` / `Blamer` / `Inspector` / `IndexManager` / `Committer` | Read/inspect and mutation capabilities |
| `Option` (`Open`/`Clone`/… variadic) | Backend and behavior configuration |
| `Err*` (e.g. `ErrRepoNotFound`, `ErrRefNotFound`, `ErrConflict`) | Typed `AppError` constructors |

Concrete backends live under `git/embedded` and `git/cli`.

---

[⬅ Back to main README](../README.md)
