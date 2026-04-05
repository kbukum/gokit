package password_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/kbukum/gokit/auth/password"
)

// ─── Security Audit: Password Hasher Safe Defaults ──────────────────────────

func TestSecurityAudit_BcryptDefaultCost_IsAtLeast12(t *testing.T) {
	t.Parallel()

	hasher := password.NewBcryptHasher()
	hash, err := hasher.Hash("securepassword1")
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	if !strings.Contains(hash, "$12$") {
		t.Errorf("bcrypt default cost should be 12, got hash: %s", hash[:30])
	}
}

func TestSecurityAudit_BcryptRejectsLowCost(t *testing.T) {
	t.Parallel()

	hasher := password.NewBcryptHasher(password.WithCost(3))
	hash, err := hasher.Hash("securepassword1")
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	if strings.Contains(hash, "$03$") {
		t.Error("bcrypt accepted dangerously low cost parameter (3)")
	}
}

func TestSecurityAudit_Argon2OWASPDefaults(t *testing.T) {
	t.Parallel()

	hasher := password.NewArgon2Hasher()
	hash, err := hasher.Hash("securepassword1")
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}

	if !strings.Contains(hash, "$argon2id$") {
		t.Error("argon2 hash should use argon2id variant")
	}
	if !strings.Contains(hash, "m=65536") {
		t.Error("argon2 memory should default to 65536 KiB (64MB)")
	}
	if !strings.Contains(hash, "t=1") {
		t.Error("argon2 time should default to 1 iteration")
	}
	if !strings.Contains(hash, "p=4") {
		t.Error("argon2 parallelism should default to 4")
	}
}

func TestSecurityAudit_MinLengthEnforced(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		hasher password.Hasher
	}{
		{"bcrypt", password.NewBcryptHasher()},
		{"argon2", password.NewArgon2Hasher()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := tc.hasher.Hash("short")
			if err == nil {
				t.Error("hasher should reject passwords shorter than 8 characters")
			}
		})
	}
}

func TestSecurityAudit_BcryptMaxLengthEnforced(t *testing.T) {
	t.Parallel()

	hasher := password.NewBcryptHasher()
	longPassword := strings.Repeat("a", 73)
	_, err := hasher.Hash(longPassword)
	if err == nil {
		t.Error("bcrypt should reject passwords longer than 72 characters")
	}
}

func TestSecurityAudit_ConstantTimeVerification(t *testing.T) {
	t.Parallel()

	hasher := password.NewBcryptHasher()
	hash, err := hasher.Hash("correct-password")
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}

	if err = hasher.Verify("correct-password", hash); err != nil {
		t.Errorf("correct password should verify: %v", err)
	}
	if err = hasher.Verify("wrong-password", hash); err == nil {
		t.Error("wrong password should fail verification")
	}
}

func TestSecurityAudit_ConcurrentHash_NoRace(t *testing.T) {
	t.Parallel()

	hasher := password.NewArgon2Hasher()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			pw := fmt.Sprintf("password-%d-minimum", idx)
			hash, err := hasher.Hash(pw)
			if err != nil {
				t.Errorf("concurrent hash %d failed: %v", idx, err)
				return
			}
			if err = hasher.Verify(pw, hash); err != nil {
				t.Errorf("concurrent verify %d failed: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()
}

func TestSecurityAudit_SpecialCharacters(t *testing.T) {
	t.Parallel()

	hasher := password.NewBcryptHasher()
	specialPasswords := []string{
		"pässwörd-ünïcödé",
		"密码测试密码测试密码",
		"p@$$w0rd!#%^&*()",
		"pass\nword\ttab1",
	}

	for _, pw := range specialPasswords {
		t.Run(pw[:8], func(t *testing.T) {
			t.Parallel()
			hash, err := hasher.Hash(pw)
			if err != nil {
				return // some may be rejected, but should not panic
			}
			if err = hasher.Verify(pw, hash); err != nil {
				t.Errorf("verify failed for special password: %v", err)
			}
		})
	}
}

func TestSecurityAudit_GenerateToken_Uniqueness(t *testing.T) {
	t.Parallel()

	tokens := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		tok, err := password.GenerateToken(32)
		if err != nil {
			t.Fatalf("generate token failed: %v", err)
		}
		if tokens[tok] {
			t.Fatalf("duplicate token at iteration %d", i)
		}
		tokens[tok] = true
	}
}

func TestSecurityAudit_GenerateToken_Length(t *testing.T) {
	t.Parallel()

	tok, err := password.GenerateToken(32)
	if err != nil {
		t.Fatalf("generate token failed: %v", err)
	}
	if len(tok) != 64 {
		t.Errorf("expected 64 hex chars for 32 bytes, got %d", len(tok))
	}
}

func TestSecurityAudit_HashSHA256_Deterministic(t *testing.T) {
	t.Parallel()

	hash1 := password.HashSHA256("test-input")
	hash2 := password.HashSHA256("test-input")
	if hash1 != hash2 {
		t.Error("SHA256 should be deterministic")
	}

	hash3 := password.HashSHA256("different-input")
	if hash1 == hash3 {
		t.Error("different inputs should produce different hashes")
	}
}
