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
| `Encrypt(plaintext string)` | Encrypt to `base64(salt || nonce || ciphertext)` |
| `Decrypt(ciphertext string)` | Decrypt from base64-encoded ciphertext |

---

[⬅ Back to main README](../README.md)
