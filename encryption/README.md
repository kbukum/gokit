# encryption

AES-256-GCM encryption and decryption service for sensitive data.

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

    // Encrypt
    ciphertext, err := svc.Encrypt("sensitive data")
    if err != nil {
        panic(err)
    }
    fmt.Println(ciphertext) // base64-encoded

    // Decrypt
    plaintext, err := svc.Decrypt(ciphertext)
    if err != nil {
        panic(err)
    }
    fmt.Println(plaintext) // "sensitive data"
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Service` | AES-256-GCM encryption/decryption service |
| `NewService(key string)` | Create service (key is SHA-256 hashed to 32 bytes) |
| `Encrypt(plaintext string)` | Encrypt to base64-encoded ciphertext |
| `Decrypt(ciphertext string)` | Decrypt from base64-encoded ciphertext |

---

[⬅ Back to main README](../README.md)
