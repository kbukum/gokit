// Package encryption provides AES-GCM encryption and decryption for
// sensitive data in gokit applications.
//
// It supports automatic key derivation from passphrases using SHA-256
// hashing, producing 256-bit keys for AES-GCM authenticated encryption.
//
// # Usage
//
//	enc, err := encryption.New("my-secret-passphrase")
//	ciphertext, err := enc.Encrypt(plaintext)
//	plaintext, err := enc.Decrypt(ciphertext)
package encryption
