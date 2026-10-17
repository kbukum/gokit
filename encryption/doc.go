// Package encryption provides authenticated encryption utilities for sensitive data in gokit applications.
//
// It supports AES-256-GCM and ChaCha20-Poly1305. Passphrases are stretched with PBKDF2-SHA256;
// random key material (WithKeyMaterial) is expanded with HKDF-SHA256. Ciphertexts are encoded as
// base64(version || algorithm || salt || nonce || ciphertext), where the version records the derivation.
//
// # Usage
//
//	enc, err := encryption.New("my-secret-passphrase")
//	ciphertext, err := enc.Encrypt(plaintext)
//	plaintext, err := enc.Decrypt(ciphertext)
package encryption
