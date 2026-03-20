# bench/storage

**Cloud storage adapter for bench results — wraps gokit/storage**

`bench/storage` implements the `bench.RunStorage` interface on top of any
`gokit/storage.Storage` provider (S3, GCS, Azure Blob, local filesystem, etc.),
so benchmark results can be persisted and queried without changing a line of
benchmarking code.

## Install

```bash
go get github.com/kbukum/gokit/bench/storage@latest
```

## Quick Start

```go
package main

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/bench"
	benchstore "github.com/kbukum/gokit/bench/storage"
	"github.com/kbukum/gokit/storage"
)

func main() {
	ctx := context.Background()

	// 1. Create a gokit/storage provider (e.g. S3, GCS, local).
	store, _ := storage.New("s3://my-bucket")

	// 2. Wrap it as bench RunStorage.
	rs := benchstore.NewProviderStorage(store,
		benchstore.WithPrefix("benchmarks/prod/"),
	)

	// 3. Use it with BenchRunner.
	runner := bench.NewBenchRunner(
		bench.WithStorage[string](rs),
		// ... metrics, etc.
	)

	// Or interact with stored results directly.
	latest, _ := rs.Latest(ctx)
	fmt.Println(latest.ID, latest.Tag)

	summaries, _ := rs.List(ctx,
		bench.WithLimit(10),
		bench.WithTagFilter("v2"),
	)
	for _, s := range summaries {
		fmt.Println(s.ID, s.Dataset, s.F1)
	}
}
```

## Key Types & Functions

| Symbol | Kind | Description |
|--------|------|-------------|
| `ProviderStorage` | struct | Implements `bench.RunStorage` using a `gokit/storage.Storage` backend |
| `NewProviderStorage(store, ...Option)` | func | Creates a `ProviderStorage` from any `storage.Storage` |
| `Option` | func | Functional option for `NewProviderStorage` |
| `WithPrefix(prefix)` | func | Sets the key prefix for stored result objects (default `"bench/"`) |

### ProviderStorage Methods

| Method | Description |
|--------|-------------|
| `Save(ctx, *RunResult) (string, error)` | Serialise and store a run result; returns the run ID |
| `Load(ctx, runID) (*RunResult, error)` | Retrieve a previously stored run by ID |
| `Latest(ctx) (*RunResult, error)` | Load the most recent run |
| `List(ctx, ...ListOption) ([]RunSummary, error)` | List stored runs with optional filters |

### List Filtering

| Option | Description |
|--------|-------------|
| `bench.WithLimit(n)` | Maximum number of results to return |
| `bench.WithTagFilter(tag)` | Filter runs by tag |
| `bench.WithDatasetFilter(name)` | Filter runs by dataset name |

## Related Packages

- [**bench**](../) — core benchmarking framework
- [**storage**](../../storage/) — underlying multi-provider storage abstraction

## License

[MIT](../../LICENSE) — Copyright (c) 2024 kbukum

[← Back to bench README](../README.md)
