# storage/s3

Amazon S3 and S3-compatible adapter for `github.com/kbukum/gokit/storage`.

The core `storage` module owns the `Storage` contract, local backend, and `FactoryRegistry`. This adapter owns the AWS SDK dependency and registers itself only when the application explicitly calls `Register`.

## Usage

```go
package main

import (
    "github.com/kbukum/gokit/storage"
    storages3 "github.com/kbukum/gokit/storage/s3"
)

func configure() (*storage.FactoryRegistry, error) {
    registry := storage.NewFactoryRegistry()
    if err := storages3.Register(registry); err != nil {
        return nil, err
    }
    return registry, nil
}
```

Importing this package has no side effects. Applications own the registry and choose the backend through configuration.

`NewStorage(ctx, cfg)` copies and validates configuration and passes the startup context to AWS configuration loading. The backend owns its HTTP client: requests are bounded at ten seconds, dialing/TLS/response headers at two seconds, connections at eight per host, and idle connections at thirty seconds. HTTPS requires TLS 1.3; redirects and implicit proxy discovery are disabled.

SDK retries are disabled, including after ambiguous upload failures. Applications own any bounded retry policy and must limit it to operations they can prove idempotent; cancellation cannot retract a write already accepted by the server. Drain callers before `Close`, which releases idle connections. `storage.Component` performs that closure during teardown.

Set both `AccessKeyFile` and `SecretKeyFile` to select mounted credentials. Each file is a bounded regular secret file of 1–1024 bytes; unreadable or invalid files fail initialization. Mounted and inline credentials cannot be mixed. With neither source configured, the AWS credential chain remains available. Do not log credential values.

`Check(ctx)` verifies authenticated access to the configured bucket with `HeadBucket`, without creating it or writing an object. Components check this capability before publishing the backend and during health probes. Missing buckets, denied access, cancellation and unavailable storage fail explicitly.

`Exists(ctx, path)` returns `false, nil` only for a not-found response. Denied access, cancellation and storage/transport failures return cause-preserving errors, never evidence that an object was deleted.

For real adapter proof, use the separate [`storage/s3/testutil`](testutil/README.md) module; the provider-neutral test utilities do not import backend SDKs.
