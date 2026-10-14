# encryption

AES-256-GCM and ChaCha20-Poly1305 encryption for sensitive data.

## Install

```bash
go get github.com/kbukum/gokit
```

## Quick Start

```go
package main

import (
    "fmt"

    "github.com/kbukum/gokit/encryption"
)

func main() {
    svc, err := encryption.NewService("my-secret-key")
    if err != nil {
        panic(err)
    }

    ciphertext, err := svc.Encrypt("sensitive data")
    if err != nil {
        panic(err)
    }
    fmt.Println(ciphertext)

    plaintext, err := svc.Decrypt(ciphertext)
    if err != nil {
        panic(err)
    }
    fmt.Println(plaintext)
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Service` | AES-256-GCM encryption/decryption service |
| `ChaCha20Service` | ChaCha20-Poly1305 encryption/decryption service |
| `NewService(key string)` | Create AES-GCM service using PBKDF2-SHA256 with a random salt per encryption |
| `NewChaCha20(key string)` | Create ChaCha20-Poly1305 service using PBKDF2-SHA256 with a random salt per encryption |
| `Encrypt(plaintext string)` | Encrypt to a versioned envelope `base64(version || algorithm || salt || nonce || ciphertext)` with the header bound as AEAD associated data |
| `Decrypt(ciphertext string)` | Decrypt from base64-encoded ciphertext |

## Mounted encryption keys

Inject a `KeySource` into the credential owner. `NewFileKeys` copies explicit `KeyFile{ID, Path}` references without startup I/O; `Key(ctx, id)` reads a bounded regular secret file of 32–1024 bytes for each operation, without a positive cache. Unknown IDs return `ENCRYPTION_KEY_NOT_FOUND`; missing/invalid mounted files return dependency-unavailable with `ENCRYPTION_KEY_UNAVAILABLE` and a preserved cause. Cancellation remains cancellation, and every failure returns no key. Values use `util.SecretString` and redact ordinary formatting/JSON; call `Expose` only to construct an encryptor.

Key identifiers are operator-assigned references, not file paths supplied by a remote caller. Keep each identifier's key material stable while ciphertext needs it. Rotation adds a new identifier and retains required old keys or performs verified re-encryption; replacing the bytes under an existing ID does not migrate existing ciphertext.

## Bind ciphertext to a record

Use `New` with `WithAssociatedData` to authenticate a canonical record identity alongside the envelope header. Decryption requires the identical binding bytes; changing the tenant, record or version fails authentication and returns no plaintext.

```go
svc, err := encryption.New(masterKey, encryption.WithAssociatedData(binding))
if err != nil {
    return err
}
ciphertext, err := svc.Encrypt(secret)
```

The option copies its input. Binding bytes are authenticated but not encrypted or stored in the envelope; obtain them from trusted record metadata and encode compound identities unambiguously. Nil/empty binding retains the ordinary envelope format. Keep the key and plaintext out of logs, URLs and exports.

---

[⬅ Back to main README](../README.md)
