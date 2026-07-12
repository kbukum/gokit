# storage

Object storage abstraction for gokit.

Core contains provider-neutral contracts, explicit `FactoryRegistry`, component integration,
content-addressable wrappers, and lean local storage. Backend SDK dependencies are isolated
in opt-in adapter modules such as `storage/s3`.

## Local default

```go
import local "github.com/kbukum/gokit/storage/local"

reg := storage.NewFactoryRegistry()
if err := local.Register(reg); err != nil {
    return err
}

store, err := storage.New(reg, storage.Config{
    Provider: storage.ProviderLocal,
    Enabled:  true,
}, &local.Config{BasePath: "./data"}, log)
```

## S3 adapter

The S3 adapter is a nested Go module, so AWS SDK dependencies do not enter core.
Importing it has no side effects; register explicitly:

```go
import storages3 "github.com/kbukum/gokit/storage/s3"

reg := storage.NewFactoryRegistry()
if err := storages3.Register(reg); err != nil {
    return err
}
```

## Supabase adapter

The Supabase adapter lives in the `storage/supabase` package. It is HTTP-only, so it stays
within the storage module (no separate `go.mod`), but it is still opt-in with no import-time
side effects — register it explicitly:

```go
import "github.com/kbukum/gokit/storage/supabase"

reg := storage.NewFactoryRegistry()
if err := supabase.Register(reg); err != nil {
    return err
}
```

If a future SDK-backed adapter is introduced, it must move to a nested adapter module before
adding that dependency.
