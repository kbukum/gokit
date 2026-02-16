# storage

Multi-provider file storage with a unified interface — supports local filesystem, S3, and Supabase.

## Install

```bash
go get github.com/kbukum/gokit/storage@latest
```

## Quick Start

```go
import (
    "github.com/kbukum/gokit/storage"
    _ "github.com/kbukum/gokit/storage/s3"       // register s3 provider
    _ "github.com/kbukum/gokit/storage/supabase"  // register supabase provider
    "github.com/kbukum/gokit/logger"
)

log := logger.New()
comp := storage.NewComponent(storage.Config{
    Enabled:  true,
    Provider: "s3",
    Bucket:   "my-bucket",
    Region:   "us-east-1",
}, log)

comp.Start(ctx)
defer comp.Stop(ctx)

s := comp.Storage()
s.Upload(ctx, "docs/file.pdf", reader)
url, _ := s.URL(ctx, "docs/file.pdf")
files, _ := s.List(ctx, "docs/")
```

## Key Types & Functions

| Symbol | Description |
|---|---|
| `Storage` | Interface — `Upload`, `Download`, `Delete`, `Exists`, `URL`, `List` |
| `FileInfo` | Path, Size, LastModified, ContentType |
| `Config` | Provider name, bucket, region, endpoint, credentials, max file size |
| `Component` | Managed lifecycle — `Start`, `Stop`, `Health` |
| `NewComponent(cfg, log)` | Create a managed storage component |
| `New(cfg, log)` | Create a `Storage` directly via registered factory |

### Providers

| Package | Factory |
|---|---|
| `storage/local` | `local.NewStorage(basePath)` |
| `storage/s3` | `s3.NewStorage(ctx, s3.Config{Region, Bucket, ...})` |
| `storage/supabase` | `supabase.NewStorage(supabase.Config{URL, Bucket, ...})` |

---

[← Back to main gokit README](../README.md)
