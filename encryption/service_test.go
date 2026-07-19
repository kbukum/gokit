package encryption

import (
	"bytes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestRoundTrip_BinaryData(t *testing.T) {
	// All 256 byte values including null bytes
	var buf []byte
	for i := 0; i < 256; i++ {
		buf = append(buf, byte(i))
	}
	plaintext := string(buf)

	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "binary-key", ac.alg)
			ct, err := enc.Encrypt(plaintext)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			got, err := enc.Decrypt(ct)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if got != plaintext {
				t.Errorf("binary round-trip mismatch: got %d bytes, want %d", len(got), len(plaintext))
			}
		})
	}
}

func TestRoundTrip_LargePayload(t *testing.T) {
	// 1 MB payload
	plaintext := strings.Repeat("A", 1<<20)

	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "large-key", ac.alg)
			ct, err := enc.Encrypt(plaintext)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			got, err := enc.Decrypt(ct)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if got != plaintext {
				t.Errorf("large payload round-trip: got len=%d, want len=%d", len(got), len(plaintext))
			}
		})
	}
}

func TestRoundTrip_ControlCharacters(t *testing.T) {
	plaintexts := []string{
		"\x00",
		"\n\r\t",
		"line1\nline2\nline3",
		"\x00\x01\x02\x03\x04\x05",
		"null\x00in\x00middle",
	}

	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "ctrl-key", ac.alg)
			for i, pt := range plaintexts {
				ct, err := enc.Encrypt(pt)
				if err != nil {
					t.Fatalf("case %d Encrypt: %v", i, err)
				}
				got, err := enc.Decrypt(ct)
				if err != nil {
					t.Fatalf("case %d Decrypt: %v", i, err)
				}
				if got != pt {
					t.Errorf("case %d: mismatch", i)
				}
			}
		})
	}
}

func TestRoundTrip_EmptyPlaintext(t *testing.T) {
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "empty-key", ac.alg)
			ct, err := enc.Encrypt("")
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			got, err := enc.Decrypt(ct)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if got != "" {
				t.Errorf("expected empty string, got %q", got)
			}
		})
	}
}

func TestRoundTrip_RepeatedCycles(t *testing.T) {
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "cycle-key", ac.alg)
			plaintext := "encrypt me many times"
			for i := 0; i < 12; i++ {
				ct, err := enc.Encrypt(plaintext)
				if err != nil {
					t.Fatalf("iteration %d Encrypt: %v", i, err)
				}
				got, err := enc.Decrypt(ct)
				if err != nil {
					t.Fatalf("iteration %d Decrypt: %v", i, err)
				}
				if got != plaintext {
					t.Fatalf("iteration %d: mismatch", i)
				}
			}
		})
	}
}

// ─── 2. Wrong key produces clear error ─────────────────────────────────

func TestWrongKey_ErrorContainsDecryptHint(t *testing.T) {
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc1 := newEncryptor(t, "correct-key", ac.alg)
			enc2 := newEncryptor(t, "wrong-key", ac.alg)

			ct, err := enc1.Encrypt("secret")
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			_, err = enc2.Decrypt(ct)
			if err == nil {
				t.Fatal("expected error decrypting with wrong key")
			}
			if !strings.Contains(err.Error(), "decrypt") {
				t.Errorf("error should mention 'decrypt', got: %v", err)
			}
		})
	}
}

func TestWrongKey_DoesNotReturnGarbage(t *testing.T) {
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc1 := newEncryptor(t, "key-a", ac.alg)
			enc2 := newEncryptor(t, "key-b", ac.alg)

			ct, _ := enc1.Encrypt("sensitive data")
			result, err := enc2.Decrypt(ct)
			if err == nil {
				t.Fatal("expected error")
			}
			// Must return empty string on failure, not partial/garbage data
			if result != "" {
				t.Errorf("expected empty result on error, got %q", result)
			}
		})
	}
}

// ─── 3. Tampered ciphertext detection ──────────────────────────────────

func TestTamperedCiphertext_FlipBit(t *testing.T) {
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "tamper-key", ac.alg)
			ct, _ := enc.Encrypt("do not tamper")
			raw, _ := base64.StdEncoding.DecodeString(ct)

			// Flip a bit in the middle of the ciphertext (past the nonce)
			midpoint := len(raw) / 2
			raw[midpoint] ^= 0x01

			tampered := base64.StdEncoding.EncodeToString(raw)
			_, err := enc.Decrypt(tampered)
			if err == nil {
				t.Error("expected error for tampered ciphertext")
			}
		})
	}
}

func TestTamperedCiphertext_FlipNonceBit(t *testing.T) {
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "nonce-tamper-key", ac.alg)
			ct, _ := enc.Encrypt("nonce tamper test")
			raw, _ := base64.StdEncoding.DecodeString(ct)

			// Flip a bit in the nonce (first byte)
			raw[0] ^= 0xFF

			tampered := base64.StdEncoding.EncodeToString(raw)
			_, err := enc.Decrypt(tampered)
			if err == nil {
				t.Error("expected error for tampered nonce")
			}
		})
	}
}

func TestTamperedCiphertext_Truncated(t *testing.T) {
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "trunc-key", ac.alg)
			ct, _ := enc.Encrypt("truncation test data")
			raw, _ := base64.StdEncoding.DecodeString(ct)

			// Remove the last 4 bytes (truncate auth tag)
			truncated := base64.StdEncoding.EncodeToString(raw[:len(raw)-4])
			_, err := enc.Decrypt(truncated)
			if err == nil {
				t.Error("expected error for truncated ciphertext")
			}
		})
	}
}

func TestTamperedCiphertext_AppendedBytes(t *testing.T) {
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "append-key", ac.alg)
			ct, _ := enc.Encrypt("append test")
			raw, _ := base64.StdEncoding.DecodeString(ct)

			// Append extra bytes
			raw = append(raw, 0xDE, 0xAD, 0xBE, 0xEF)
			modified := base64.StdEncoding.EncodeToString(raw)
			_, err := enc.Decrypt(modified)
			if err == nil {
				t.Error("expected error for ciphertext with appended bytes")
			}
		})
	}
}

func TestTamperedCiphertext_FlipAuthTag(t *testing.T) {
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "tag-key", ac.alg)
			ct, _ := enc.Encrypt("auth tag test")
			raw, _ := base64.StdEncoding.DecodeString(ct)

			// Flip a bit in the last byte (part of the auth tag)
			raw[len(raw)-1] ^= 0x01
			tampered := base64.StdEncoding.EncodeToString(raw)
			_, err := enc.Decrypt(tampered)
			if err == nil {
				t.Error("expected error for tampered auth tag")
			}
		})
	}
}

// ─── 4. Key rotation pattern ───────────────────────────────────────────

func TestKeyRotation_OldCiphertextDecryptableByOldKey(t *testing.T) {
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			oldEnc := newEncryptor(t, "old-key-v1", ac.alg)
			newEnc := newEncryptor(t, "new-key-v2", ac.alg)

			// Encrypt with old key
			ct, _ := oldEnc.Encrypt("legacy data")

			// New key can't decrypt
			_, err := newEnc.Decrypt(ct)
			if err == nil {
				t.Fatal("new key should not decrypt old ciphertext")
			}

			// Old key still works
			got, err := oldEnc.Decrypt(ct)
			if err != nil {
				t.Fatalf("old key should still decrypt: %v", err)
			}
			if got != "legacy data" {
				t.Errorf("got %q, want %q", got, "legacy data")
			}
		})
	}
}

func TestKeyRotation_FallbackPattern(t *testing.T) {
	// Demonstrates the recommended key rotation approach:
	// try the current key first, fall back to previous keys.
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			keys := []string{"key-v1", "key-v2", "key-v3"}
			encryptors := make([]Encryptor, 0, len(keys))
			for _, k := range keys {
				encryptors = append(encryptors, newEncryptor(t, k, ac.alg))
			}

			// Encrypt with v1
			ct, _ := encryptors[0].Encrypt("rotate me")

			// tryDecrypt tries keys in reverse order (newest first)
			tryDecrypt := func(ciphertext string) (string, error) {
				for i := len(encryptors) - 1; i >= 0; i-- {
					if pt, err := encryptors[i].Decrypt(ciphertext); err == nil {
						return pt, nil
					}
				}
				return "", fmt.Errorf("all keys failed")
			}

			got, err := tryDecrypt(ct)
			if err != nil {
				t.Fatalf("fallback decryption failed: %v", err)
			}
			if got != "rotate me" {
				t.Errorf("got %q, want %q", got, "rotate me")
			}
		})
	}
}

func TestDecryptRejectsPayloadWithoutNonce(t *testing.T) {
	t.Parallel()

	raw := make([]byte, saltSize)
	ciphertext := base64.StdEncoding.EncodeToString(raw)

	for _, tc := range []struct {
		name string
		new  func(string) (Encryptor, error)
	}{
		{name: "aes", new: func(key string) (Encryptor, error) { return NewService(key) }},
		{name: "chacha20", new: func(key string) (Encryptor, error) { return NewChaCha20(key) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			enc, err := tc.new("missing-nonce")
			if err != nil {
				t.Fatalf("new encryptor: %v", err)
			}
			_, err = enc.Decrypt(ciphertext)
			if err == nil || !strings.Contains(err.Error(), "ciphertext too short") {
				t.Fatalf("Decrypt() error = %v, want ciphertext too short", err)
			}
		})
	}
}

func TestAEADFactoriesRejectInvalidKeyMaterial(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		factory aeadFactory
	}{
		{name: "aes", factory: newAESGCM},
		{name: "chacha20", factory: newChaCha20Poly1305},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := tc.factory([]byte("short")); err == nil {
				t.Fatal("expected invalid key material to be rejected")
			}
		})
	}
}

func TestEncryptWithAEADPropagatesFactoryError(t *testing.T) {
	t.Parallel()

	want := errors.New("factory unavailable")
	_, err := encryptWithAEAD([]byte("key"), func([]byte) (cipher.AEAD, error) {
		return nil, want
	}, "secret")
	if !errors.Is(err, want) {
		t.Fatalf("Encrypt error = %v, want %v", err, want)
	}
}

func FuzzDecryptRejectsTamperedPayload(f *testing.F) {
	svc, err := NewService("fuzz-key")
	if err != nil {
		f.Fatalf("NewService: %v", err)
	}
	ciphertext, err := svc.Encrypt("authenticated plaintext")
	if err != nil {
		f.Fatalf("Encrypt: %v", err)
	}
	valid, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		f.Fatalf("DecodeString: %v", err)
	}

	f.Add([]byte{0})
	f.Add([]byte{1, 2, 3, 4})
	f.Add([]byte("tamper"))
	f.Fuzz(func(t *testing.T, mutation []byte) {
		tampered := append([]byte(nil), valid...)
		tampered[0] ^= 0x80
		for i, b := range mutation {
			tampered[i%len(tampered)] ^= b
		}

		// The mutation can XOR the payload back to its original bytes; an
		// unchanged ciphertext is not a tampered one, so skip that case.
		if bytes.Equal(tampered, valid) {
			return
		}

		if plaintext, err := svc.Decrypt(base64.StdEncoding.EncodeToString(tampered)); err == nil {
			t.Fatalf("tampered ciphertext decrypted to %q", plaintext)
		}
	})
}

// ─── 5. Factory pattern (New + WithAlgorithm) ─────────────────────────

func TestSecurity_VariousKeyLengths(t *testing.T) {
	// PBKDF2-SHA256 stretches passphrases, so any length should work.
	keys := []string{
		"",                        // empty key
		"a",                       // 1 byte
		"short",                   // 5 bytes
		strings.Repeat("x", 16),   // 16 bytes (AES-128 length)
		strings.Repeat("x", 32),   // 32 bytes (AES-256 length)
		strings.Repeat("x", 64),   // 64 bytes
		strings.Repeat("x", 1024), // 1 KB key
	}

	for _, ac := range algorithms {
		for i, key := range keys {
			t.Run(fmt.Sprintf("%s/keyLen=%d", ac.name, len(key)), func(t *testing.T) {
				enc := newEncryptor(t, key, ac.alg)
				ct, err := enc.Encrypt("test")
				if err != nil {
					t.Fatalf("key %d Encrypt: %v", i, err)
				}
				pt, err := enc.Decrypt(ct)
				if err != nil {
					t.Fatalf("key %d Decrypt: %v", i, err)
				}
				if pt != "test" {
					t.Errorf("key %d: got %q", i, pt)
				}
			})
		}
	}
}

func TestSecurity_NonceUniqueness(t *testing.T) {
	// Encrypt the same plaintext many times and ensure all ciphertexts are unique.
	// This verifies nonces are generated randomly and not reused.
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "nonce-test-key", ac.alg)
			seen := make(map[string]bool, 32)
			for i := 0; i < 32; i++ {
				ct, err := enc.Encrypt("same plaintext")
				if err != nil {
					t.Fatalf("iteration %d: %v", i, err)
				}
				if seen[ct] {
					t.Fatalf("duplicate ciphertext at iteration %d (nonce reuse!)", i)
				}
				seen[ct] = true
			}
		})
	}
}

func TestSecurity_CiphertextDoesNotContainPlaintext(t *testing.T) {
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "no-leak-key", ac.alg)
			plaintext := "SUPER_SECRET_VALUE_12345"
			ct, _ := enc.Encrypt(plaintext)

			// Ciphertext is base64 — decode and check raw bytes too
			raw, _ := base64.StdEncoding.DecodeString(ct)

			if strings.Contains(ct, plaintext) {
				t.Error("base64 ciphertext contains plaintext")
			}
			if strings.Contains(string(raw), plaintext) {
				t.Error("raw ciphertext bytes contain plaintext")
			}
		})
	}
}

func TestSecurity_DifferentKeySamePassphrase_DifferentResult(t *testing.T) {
	// Verify that keys that differ by one character produce different ciphertexts
	// that can't be decrypted by each other.
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc1 := newEncryptor(t, "passphrase-a", ac.alg)
			enc2 := newEncryptor(t, "passphrase-b", ac.alg)

			ct1, _ := enc1.Encrypt("test")
			ct2, _ := enc2.Encrypt("test")

			// Cross-decryption must fail
			if _, err := enc1.Decrypt(ct2); err == nil {
				t.Error("enc1 should not decrypt enc2's ciphertext")
			}
			if _, err := enc2.Decrypt(ct1); err == nil {
				t.Error("enc2 should not decrypt enc1's ciphertext")
			}
		})
	}
}

func TestSecurity_SameKeySameAlgorithm_Interoperable(t *testing.T) {
	// Two instances created with the same key should interoperate.
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc1 := newEncryptor(t, "shared-key-42", ac.alg)
			enc2 := newEncryptor(t, "shared-key-42", ac.alg)

			ct, _ := enc1.Encrypt("hello from enc1")
			pt, err := enc2.Decrypt(ct)
			if err != nil {
				t.Fatalf("same key decrypt: %v", err)
			}
			if pt != "hello from enc1" {
				t.Errorf("got %q", pt)
			}
		})
	}
}

// ─── 7. Edge cases ─────────────────────────────────────────────────────

func TestEdge_DecryptEmptyString(t *testing.T) {
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "edge-key", ac.alg)
			_, err := enc.Decrypt("")
			if err == nil {
				t.Error("expected error decrypting empty string")
			}
		})
	}
}

func TestEdge_DecryptRandomString(t *testing.T) {
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "edge-key", ac.alg)
			_, err := enc.Decrypt("aGVsbG8gd29ybGQ=") // "hello world" in base64 — not a valid ciphertext
			if err == nil {
				t.Error("expected error decrypting non-ciphertext base64")
			}
		})
	}
}

func TestEdge_VeryLongKey(t *testing.T) {
	longKey := strings.Repeat("k", 1<<16) // 64 KB key
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, longKey, ac.alg)
			ct, err := enc.Encrypt("long key test")
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			pt, err := enc.Decrypt(ct)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if pt != "long key test" {
				t.Errorf("got %q", pt)
			}
		})
	}
}

func TestEdge_UnicodeMultibyte(t *testing.T) {
	plaintexts := []string{
		"🔐🔑🛡️",               // emoji
		"Ω≈ç√∫≤≥÷",           // math symbols
		"日本語テスト",             // Japanese
		"مرحبا",              // Arabic
		"Привет",             // Cyrillic
		"\u0000\u200B\uFEFF", // null + zero-width space + BOM
		"mixed 🎉 content\nwith\ttabs\x00and\nnulls",
	}

	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "unicode-key", ac.alg)
			for _, pt := range plaintexts {
				ct, err := enc.Encrypt(pt)
				if err != nil {
					t.Fatalf("Encrypt %q: %v", pt, err)
				}
				got, err := enc.Decrypt(ct)
				if err != nil {
					t.Fatalf("Decrypt %q: %v", pt, err)
				}
				if got != pt {
					t.Errorf("got %q, want %q", got, pt)
				}
			}
		})
	}
}

func TestEdge_CiphertextIsValidBase64(t *testing.T) {
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "b64-key", ac.alg)
			ct, _ := enc.Encrypt("base64 check")
			_, err := base64.StdEncoding.DecodeString(ct)
			if err != nil {
				t.Errorf("ciphertext should be valid base64: %v", err)
			}
		})
	}
}

func TestEdge_DecryptSwappedCiphertexts(t *testing.T) {
	// Two different plaintexts — ensure you can't swap their ciphertexts
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "swap-key", ac.alg)
			ct1, _ := enc.Encrypt("plaintext-A")
			ct2, _ := enc.Encrypt("plaintext-B")

			pt1, _ := enc.Decrypt(ct1)
			pt2, _ := enc.Decrypt(ct2)

			if pt1 != "plaintext-A" {
				t.Errorf("ct1 should decrypt to A, got %q", pt1)
			}
			if pt2 != "plaintext-B" {
				t.Errorf("ct2 should decrypt to B, got %q", pt2)
			}
		})
	}
}

// ─── 8. Concurrent encrypt/decrypt safety ──────────────────────────────

func TestConcurrency_ParallelEncrypt(t *testing.T) {
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "concurrent-key", ac.alg)
			const goroutines = 8
			var wg sync.WaitGroup
			errs := make(chan error, goroutines)

			for i := 0; i < goroutines; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					pt := fmt.Sprintf("goroutine-%d", id)
					ct, err := enc.Encrypt(pt)
					if err != nil {
						errs <- fmt.Errorf("goroutine %d encrypt: %w", id, err)
						return
					}
					got, err := enc.Decrypt(ct)
					if err != nil {
						errs <- fmt.Errorf("goroutine %d decrypt: %w", id, err)
						return
					}
					if got != pt {
						errs <- fmt.Errorf("goroutine %d: got %q, want %q", id, got, pt)
					}
				}(i)
			}

			wg.Wait()
			close(errs)
			for err := range errs {
				t.Error(err)
			}
		})
	}
}

func TestConcurrency_ParallelDecrypt(t *testing.T) {
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "concurrent-dec-key", ac.alg)

			// Pre-encrypt test data
			const count = 12
			type pair struct {
				pt, ct string
			}
			pairs := make([]pair, count)
			for i := 0; i < count; i++ {
				pt := fmt.Sprintf("data-%d", i)
				ct, err := enc.Encrypt(pt)
				if err != nil {
					t.Fatalf("pre-encrypt %d: %v", i, err)
				}
				pairs[i] = pair{pt, ct}
			}

			// Decrypt all in parallel
			var wg sync.WaitGroup
			errs := make(chan error, count)
			for i := 0; i < count; i++ {
				wg.Add(1)
				go func(p pair) {
					defer wg.Done()
					got, err := enc.Decrypt(p.ct)
					if err != nil {
						errs <- fmt.Errorf("decrypt %q: %w", p.pt, err)
						return
					}
					if got != p.pt {
						errs <- fmt.Errorf("got %q, want %q", got, p.pt)
					}
				}(pairs[i])
			}

			wg.Wait()
			close(errs)
			for err := range errs {
				t.Error(err)
			}
		})
	}
}

func TestConcurrency_MixedEncryptDecrypt(t *testing.T) {
	for _, ac := range algorithms {
		t.Run(ac.name, func(t *testing.T) {
			enc := newEncryptor(t, "mixed-key", ac.alg)
			const goroutines = 16
			var wg sync.WaitGroup
			errs := make(chan error, goroutines)

			for i := 0; i < goroutines; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					pt := fmt.Sprintf("mixed-%d", id)

					// Encrypt
					ct, err := enc.Encrypt(pt)
					if err != nil {
						errs <- fmt.Errorf("encrypt %d: %w", id, err)
						return
					}

					// Immediately decrypt
					got, err := enc.Decrypt(ct)
					if err != nil {
						errs <- fmt.Errorf("decrypt %d: %w", id, err)
						return
					}
					if got != pt {
						errs <- fmt.Errorf("id %d: got %q, want %q", id, got, pt)
					}
				}(i)
			}

			wg.Wait()
			close(errs)
			for err := range errs {
				t.Error(err)
			}
		})
	}
}
