// Package encryption provides authenticated encryption utilities for sensitive data in gokit applications.
//
// It supports AES-256-GCM and ChaCha20-Poly1305 with PBKDF2-SHA256 key derivation,
// producing ciphertexts encoded as base64(salt || nonce || ciphertext).
//
// # Usage
//
// enc, err := encryption.New("my-secret-passphrase")
// ciphertext, err := enc.Encrypt(plaintext)
// plaintext, err := enc.Decrypt(ciphertext)
package encryption
