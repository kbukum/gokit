# encryption

Symmetric encryption for gokit applications with AES-256-GCM and ChaCha20-Poly1305.

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
    // Default: AES-256-GCM
    enc, err := encryption.New("my-secret-passphrase")
    if err != nil {
        panic(err)
    }

    ciphertext, err := enc.Encrypt("sensitive data")
    if err != nil {
        panic(err)
    }
    fmt.Println(ciphertext) // base64-encoded

    plaintext, err := enc.Decrypt(ciphertext)
    if err != nil {
        panic(err)
    }
    fmt.Println(plaintext) // "sensitive data"
}
```

## Algorithms

| Algorithm | Constant | Best For |
|-----------|----------|----------|
| AES-256-GCM (default) | `AlgorithmAESGCM` | CPUs with AES-NI hardware acceleration |
| ChaCha20-Poly1305 | `AlgorithmChaCha20` | CPUs without AES-NI (ARM, older x86) |

```go
// Explicit algorithm selection
enc, err := encryption.New("key", encryption.WithAlgorithm(encryption.AlgorithmChaCha20))
```

## Key Derivation

Keys are derived from passphrases using **PBKDF2-SHA256** with:
- **600,000 iterations** (OWASP 2023 recommendation)
- **Random 16-byte salt** per encryption operation

## Ciphertext Format

```
base64(salt[16] || nonce[12] || ciphertext)
```

The salt is prepended to the ciphertext so decryption can extract it and re-derive the key.

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Encryptor` | Interface: `Encrypt(string) (string, error)` and `Decrypt(string) (string, error)` |
| `New(key, ...Option)` | Factory: creates an `Encryptor` for the chosen algorithm |
| `WithAlgorithm(alg)` | Option: selects encryption algorithm |
| `Service` | AES-256-GCM implementation |
| `ChaCha20Service` | ChaCha20-Poly1305 implementation |

## Security Considerations

- Each encryption generates a unique random salt and nonce
- PBKDF2 with 600k iterations resists brute-force and rainbow table attacks
- Both algorithms provide authenticated encryption (AEAD)
- The same plaintext encrypted twice produces different ciphertext

---

[⬅ Back to main README](../README.md)
