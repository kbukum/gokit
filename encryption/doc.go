// Package encryption provides symmetric encryption and decryption for
// sensitive data in gokit applications.
//
// It supports AES-256-GCM (default) and ChaCha20-Poly1305 algorithms with
// PBKDF2-SHA256 key derivation (600,000 iterations, random 16-byte salt).
//
// Ciphertext format: base64(salt[16] || nonce[12] || ciphertext)
//
// # Usage
//
//	enc, err := encryption.New("my-secret-passphrase")
//	ciphertext, err := enc.Encrypt(plaintext)
//	plaintext, err := enc.Decrypt(ciphertext)
package encryption
