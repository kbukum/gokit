# storage

Object storage abstraction for gokit.

Core contains provider-neutral contracts, explicit `FactoryRegistry`, component integration, content-addressable wrappers, and lean local storage. Backend SDK dependencies are isolated in opt-in adapter modules such as `storage/s3`.

## Lifecycle injection

Inject `Component` directly as `Storage`, or use `Component.Source` when a consumer needs the underlying backend. Operations return a typed unavailable error until successful startup and after teardown, without waiting for provider initialization. Factories receive the startup context; canceled or failed startup never publishes a backend. A backend returned alongside an initialization error is closed with an independent two-second cleanup deadline.

Stop acceptance and drain active operations before stopping the component. `Stop` cancels and waits for initialization, unpublishes the backend, and invokes its optional `Closer` under the caller's deadline. Failed startup cleanup or teardown retains an unavailable backend for a later `Stop` retry; restart is refused until cleanup succeeds. Closers must be safe to retry and return nil only after complete release. Overlapping startup is rejected during initialization or teardown; concurrent stops wait for the same teardown result within their own deadlines. A caller constructing a backend with `New` owns any non-nil returned backend, including when initialization also returns an error.

Backends may implement `Checker` for authenticated readiness without mutation. The component checks it before startup publication and on every health probe; failed readiness closes the acquired backend rather than making it available. Backends without this capability retain the URL-based health check.

## Local default

```go
import local "github.com/kbukum/gokit/storage/local"

reg := storage.NewFactoryRegistry()
if err := local.Register(reg, local.Config{BasePath: "./data"}); err != nil {
    return err
}

store, err := storage.New(ctx, reg, storage.Config{
    Provider: storage.ProviderLocal,
    Enabled:  true,
}, log)
```

## S3 adapter

The S3 adapter is a nested Go module, so AWS SDK dependencies do not enter core.
Importing it has no side effects; register explicitly:

```go
import (
    "github.com/kbukum/gokit/storage"
    storages3 "github.com/kbukum/gokit/storage/s3"
)

reg := storage.NewFactoryRegistry()
if err := storages3.Register(reg); err != nil {
    return err
}
```

## Supabase adapter

The Supabase adapter lives in the `storage/supabase` package. It is HTTP-only, so it stays within the storage module (no separate `go.mod`), but it is still opt-in with no import-time side effects — register it explicitly:

```go
import (
    "github.com/kbukum/gokit/storage"
    "github.com/kbukum/gokit/storage/supabase"
)

reg := storage.NewFactoryRegistry()
if err := supabase.Register(reg); err != nil {
    return err
}
```

If a future SDK-backed adapter is introduced, it must move to a nested adapter module before adding that dependency.
