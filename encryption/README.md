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
| `New(key, opts...)` | Create either algorithm; `WithAlgorithm`, `WithAssociatedData` and `WithKeyMaterial` configure it |
| `WithKeyMaterial()` | Treat the key as random key material of at least 32 bytes and derive message keys with HKDF-SHA256 (envelope version 2) |
| `Encrypt(plaintext string)` | Encrypt to a versioned envelope `base64(version || algorithm || salt || nonce || ciphertext)` with the header bound as AEAD associated data |
| `Decrypt(ciphertext string)` | Decrypt from base64-encoded ciphertext |

## Passphrases and key material

A passphrase chosen by a person needs stretching, so the default derives each message key with PBKDF2-SHA256 at 600,000 iterations: tens of milliseconds of CPU per operation, by design. A random key, such as a mounted key file, needs no stretching. Pass `WithKeyMaterial()` and each message key comes from HKDF-SHA256 over the key and the envelope's random salt, with an algorithm-specific info string, in microseconds. `New` refuses key material shorter than 32 bytes.

The envelope version records the derivation: 1 for passphrases, 2 for key material. An encryptor opens only its own version, so a passphrase can never be used against key-material ciphertext or the reverse. rskit-encryption produces and opens the same envelopes; both kits test the same vectors.

## Mounted encryption keys

Inject a `KeySource` into the credential owner. `NewFileKeys` copies explicit `KeyFile{ID, Path}` references without startup I/O; `Key(ctx, id)` reads a bounded regular secret file of 32–1024 bytes for each operation, without a positive cache. Unknown IDs return `ENCRYPTION_KEY_NOT_FOUND`; missing/invalid mounted files return dependency-unavailable with `ENCRYPTION_KEY_UNAVAILABLE` and a preserved cause. Cancellation remains cancellation, and every failure returns no key. Values use `util.SecretString` and redact ordinary formatting/JSON; call `Expose` only to construct an encryptor, with `WithKeyMaterial()`.

Key identifiers are operator-assigned references, not file paths supplied by a remote caller. Keep each identifier's key material stable while ciphertext needs it. Rotation adds a new identifier and retains required old keys or performs verified re-encryption; replacing the bytes under an existing ID does not migrate existing ciphertext.

## Bind ciphertext to a record

Use `New` with `WithAssociatedData` to authenticate a canonical record identity alongside the envelope header. Decryption requires the identical binding bytes; changing the tenant, record or version fails authentication and returns no plaintext.

```go
svc, err := encryption.New(masterKey, encryption.WithKeyMaterial(), encryption.WithAssociatedData(binding))
if err != nil {
    return err
}
ciphertext, err := svc.Encrypt(secret)
```

The option copies its input. Binding bytes are authenticated but not encrypted or stored in the envelope; obtain them from trusted record metadata and encode compound identities unambiguously. Nil/empty binding retains the ordinary envelope format. Keep the key and plaintext out of logs, URLs and exports.

---

[⬅ Back to main README](../README.md)
