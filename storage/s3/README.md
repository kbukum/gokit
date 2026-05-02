# storage/s3

Amazon S3 and S3-compatible adapter for `github.com/kbukum/gokit/storage`.

The core `storage` module owns the `Storage` contract, local backend, and
`FactoryRegistry`. This adapter owns the AWS SDK dependency and registers itself
only when the application explicitly calls `Register`.

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

Importing this package has no side effects. Applications own the registry and
choose the backend through configuration.
