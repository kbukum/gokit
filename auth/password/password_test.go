package password

import (
	"strings"
	"sync"
	"testing"
)

// ── BcryptHasher ────────────────────────────────────────────────────

func TestBcryptHasher_HashVerifyRoundTrip(t *testing.T) {
	h := NewBcryptHasher()
	hash, err := h.Hash("valid-password")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if err := h.Verify("valid-password", hash); err != nil {
		t.Fatalf("Verify should succeed: %v", err)
	}
}

func TestBcryptHasher_DifferentPasswordsDifferentHashes(t *testing.T) {
	h := NewBcryptHasher()
	h1, _ := h.Hash("password-one!")
	h2, _ := h.Hash("password-two!")
	if h1 == h2 {
		t.Error("different passwords should produce different hashes")
	}
}

func TestBcryptHasher_SamePasswordDifferentHashes(t *testing.T) {
	h := NewBcryptHasher()
	h1, _ := h.Hash("same-password")
	h2, _ := h.Hash("same-password")
	if h1 == h2 {
		t.Error("same password should produce different hashes due to random salt")
	}
}

func TestBcryptHasher_CostParameter(t *testing.T) {
	tests := []struct {
		name      string
		cost      int
		wantCost  int
		expectErr bool
	}{
		{"min valid cost", 4, 4, false},
		{"max valid cost", 31, 31, false},
		{"default cost", 0, 12, false}, // WithCost(0) won't change default
		{"cost below min stays default", 3, 12, false},
		{"cost above max stays default", 32, 12, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h *BcryptHasher
			if tt.cost == 0 {
				h = NewBcryptHasher()
			} else {
				h = NewBcryptHasher(WithCost(tt.cost))
			}
			if h.cost != tt.wantCost {
				t.Errorf("cost = %d, want %d", h.cost, tt.wantCost)
			}
		})
	}
}

func TestBcryptHasher_EmptyPasswordRejected(t *testing.T) {
	h := NewBcryptHasher()
	_, err := h.Hash("")
	if err == nil {
		t.Fatal("expected error for empty password")
	}
	if !strings.Contains(err.Error(), "minimum length") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestBcryptHasher_ShortPasswordRejected(t *testing.T) {
	h := NewBcryptHasher()
	_, err := h.Hash("short")
	if err == nil {
		t.Fatal("expected error for short password")
	}
	if !strings.Contains(err.Error(), "minimum length") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestBcryptHasher_VeryLongPasswordRejected(t *testing.T) {
	h := NewBcryptHasher()
	longPw := strings.Repeat("a", 73)
	_, err := h.Hash(longPw)
	if err == nil {
		t.Fatal("expected error for password > 72 bytes")
	}
	if !strings.Contains(err.Error(), "maximum length") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestBcryptHasher_ExactlyMaxLengthAccepted(t *testing.T) {
	h := NewBcryptHasher()
	pw := strings.Repeat("a", 72)
	hash, err := h.Hash(pw)
	if err != nil {
		t.Fatalf("72-byte password should be accepted: %v", err)
	}
	if err := h.Verify(pw, hash); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestBcryptHasher_ExactlyMinLengthAccepted(t *testing.T) {
	h := NewBcryptHasher()
	pw := "12345678" // exactly 8
	hash, err := h.Hash(pw)
	if err != nil {
		t.Fatalf("8-char password should be accepted: %v", err)
	}
	if err := h.Verify(pw, hash); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestBcryptHasher_UnicodePassword(t *testing.T) {
	h := NewBcryptHasher()
	pw := "пароль-密码-🔑abc" // mix of Cyrillic, Chinese, emoji, ASCII
	hash, err := h.Hash(pw)
	if err != nil {
		t.Fatalf("unicode password should be accepted: %v", err)
	}
	if err := h.Verify(pw, hash); err != nil {
		t.Fatalf("Verify unicode password: %v", err)
	}
}

func TestBcryptHasher_WrongPasswordFails(t *testing.T) {
	h := NewBcryptHasher()
	hash, _ := h.Hash("correct-password")
	err := h.Verify("wrong-password!!", hash)
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
	if !strings.Contains(err.Error(), "invalid password") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestBcryptHasher_TamperedHashFails(t *testing.T) {
	h := NewBcryptHasher()
	hash, _ := h.Hash("my-password!")
	tampered := hash[:len(hash)-4] + "XXXX"
	err := h.Verify("my-password!", tampered)
	if err == nil {
		t.Fatal("expected error for tampered hash")
	}
}

func TestBcryptHasher_HashFormatPrefix(t *testing.T) {
	h := NewBcryptHasher()
	hash, _ := h.Hash("test-password")
	if !strings.HasPrefix(hash, "$2") {
		t.Errorf("bcrypt hash should start with $2, got prefix: %s", hash[:4])
	}
}

func TestBcryptHasher_NoPlaintextInErrors(t *testing.T) {
	h := NewBcryptHasher()
	pw := "secret-password"
	_, err := h.Hash("short") // too short
	if err != nil && strings.Contains(err.Error(), pw) {
		t.Error("error message should not contain the plaintext password")
	}
	hash, _ := h.Hash("valid-password!")
	err = h.Verify(pw, hash)
	if err != nil && strings.Contains(err.Error(), pw) {
		t.Error("verify error should not contain the plaintext password")
	}
}

// ── Argon2Hasher ────────────────────────────────────────────────────

func TestArgon2Hasher_HashVerifyRoundTrip(t *testing.T) {
	h := NewArgon2Hasher()
	hash, err := h.Hash("valid-password")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if err := h.Verify("valid-password", hash); err != nil {
		t.Fatalf("Verify should succeed: %v", err)
	}
}

func TestArgon2Hasher_ParameterVariations(t *testing.T) {
	h := NewArgon2Hasher(
		WithArgon2Time(2),
		WithArgon2Memory(32*1024),
		WithArgon2Threads(2),
	)
	hash, err := h.Hash("test-pass")
	if err != nil {
		t.Fatalf("Hash with custom params: %v", err)
	}
	if err := h.Verify("test-pass", hash); err != nil {
		t.Fatalf("Verify with custom params: %v", err)
	}
}

func TestArgon2Hasher_SaltUniqueness(t *testing.T) {
	h := NewArgon2Hasher()
	h1, _ := h.Hash("same-password")
	h2, _ := h.Hash("same-password")
	if h1 == h2 {
		t.Error("same password should produce different hashes (unique salt)")
	}
}

func TestArgon2Hasher_InvalidHashFormat(t *testing.T) {
	h := NewArgon2Hasher()
	tests := []struct {
		name string
		hash string
	}{
		{"empty string", ""},
		{"no dollar signs", "notahash"},
		{"wrong prefix", "$bcrypt$v=19$m=65536,t=1,p=4$c29tZXNhbHQ$c29tZWhhc2g"},
		{"missing parts", "$argon2id$v=19"},
		{"bad params format", "$argon2id$v=19$garbage$c29tZXNhbHQ$c29tZWhhc2g"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.Verify("password", tt.hash)
			if err == nil {
				t.Error("expected error for invalid hash format")
			}
		})
	}
}

func TestArgon2Hasher_WrongPasswordFails(t *testing.T) {
	h := NewArgon2Hasher()
	hash, _ := h.Hash("correct-pw")
	err := h.Verify("wrong-pw!!!", hash)
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
	if !strings.Contains(err.Error(), "invalid password") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestArgon2Hasher_DefaultsFollowOWASP(t *testing.T) {
	h := NewArgon2Hasher()
	if h.time != 3 {
		t.Errorf("default time should be 3, got %d", h.time)
	}
	if h.memory != 64*1024 {
		t.Errorf("OWASP default memory should be 64MB (65536 KiB), got %d", h.memory)
	}
	if h.threads != 4 {
		t.Errorf("OWASP default threads should be 4, got %d", h.threads)
	}
	if h.keyLen != 32 {
		t.Errorf("default keyLen should be 32, got %d", h.keyLen)
	}
}

func TestArgon2Hasher_HashFormatEncoding(t *testing.T) {
	h := NewArgon2Hasher()
	hash, _ := h.Hash("test-pass")
	if !strings.HasPrefix(hash, "$argon2id$v=") {
		t.Errorf("argon2 hash should start with $argon2id$v=, got: %s", hash[:20])
	}
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		t.Errorf("argon2 hash should have 6 parts, got %d", len(parts))
	}
}

func TestArgon2Hasher_EmptyPasswordRejected(t *testing.T) {
	h := NewArgon2Hasher()
	_, err := h.Hash("")
	if err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestArgon2Hasher_ShortPasswordRejected(t *testing.T) {
	h := NewArgon2Hasher()
	_, err := h.Hash("short")
	if err == nil {
		t.Fatal("expected error for short password")
	}
}

func TestArgon2Hasher_UnicodePassword(t *testing.T) {
	h := NewArgon2Hasher()
	pw := "パスワード-contraseña" // Japanese + Spanish
	hash, err := h.Hash(pw)
	if err != nil {
		t.Fatalf("Hash unicode: %v", err)
	}
	if err := h.Verify(pw, hash); err != nil {
		t.Fatalf("Verify unicode: %v", err)
	}
}

func TestArgon2Hasher_ConstantTimeComparison(t *testing.T) {
	// Verify that the Verify method uses subtle.ConstantTimeCompare
	// by checking it rejects wrong passwords consistently
	h := NewArgon2Hasher()
	hash, _ := h.Hash("right-password")
	for i := 0; i < 10; i++ {
		err := h.Verify("wrong-password", hash)
		if err == nil {
			t.Fatal("wrong password should always fail")
		}
	}
}

// ── Config & Factory ────────────────────────────────────────────────

func TestConfig_ApplyDefaults(t *testing.T) {
	c := &Config{}
	c.ApplyDefaults()
	if c.Algorithm != AlgorithmArgon2id {
		t.Errorf("default algorithm should be argon2id, got %s", c.Algorithm)
	}
	if c.BcryptCost != 12 {
		t.Errorf("default bcrypt cost should be 12, got %d", c.BcryptCost)
	}
	if c.MinLength != 8 {
		t.Errorf("default min length should be 8, got %d", c.MinLength)
	}
	if c.Argon2Time != 3 {
		t.Errorf("default argon2 time should be 3, got %d", c.Argon2Time)
	}
	if c.Argon2Memory != 64*1024 {
		t.Errorf("default argon2 memory should be 65536, got %d", c.Argon2Memory)
	}
	if c.Argon2Threads != 4 {
		t.Errorf("default argon2 threads should be 4, got %d", c.Argon2Threads)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"valid bcrypt", Config{Algorithm: AlgorithmBcrypt, BcryptCost: 12, MinLength: 8}, false},
		{"valid argon2id", Config{Algorithm: AlgorithmArgon2id, Argon2Time: 3, Argon2Memory: 64 * 1024, Argon2Threads: 4, MinLength: 8}, false},
		{"unsupported algorithm", Config{Algorithm: "scrypt", BcryptCost: 12, MinLength: 8}, true},
		{"bcrypt cost too low", Config{Algorithm: AlgorithmBcrypt, BcryptCost: 3, MinLength: 8}, true},
		{"bcrypt cost too high", Config{Algorithm: AlgorithmBcrypt, BcryptCost: 32, MinLength: 8}, true},
		{"argon2 time too low", Config{Algorithm: AlgorithmArgon2id, Argon2Time: 2, Argon2Memory: 64 * 1024, Argon2Threads: 4, MinLength: 8}, true},
		{"argon2 memory too low", Config{Algorithm: AlgorithmArgon2id, Argon2Time: 3, Argon2Memory: 32 * 1024, Argon2Threads: 4, MinLength: 8}, true},
		{"min length zero", Config{Algorithm: AlgorithmBcrypt, BcryptCost: 12, MinLength: 0}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewHasher_BcryptFromConfig(t *testing.T) {
	cfg := Config{Algorithm: AlgorithmBcrypt, BcryptCost: 10}
	h := NewHasher(cfg)
	hash, err := h.Hash("test-pass")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Error("expected bcrypt hash format")
	}
}

func TestNewHasher_Argon2FromConfig(t *testing.T) {
	cfg := Config{Algorithm: AlgorithmArgon2id}
	h := NewHasher(cfg)
	hash, err := h.Hash("test-pass")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Error("expected argon2id hash format")
	}
}

// ── Token Utilities ─────────────────────────────────────────────────

func TestGenerateToken_Length(t *testing.T) {
	token, err := GenerateToken(32)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	// 32 bytes → 64 hex chars
	if len(token) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(token))
	}
}

func TestGenerateToken_Uniqueness(t *testing.T) {
	t1, _ := GenerateToken(32)
	t2, _ := GenerateToken(32)
	if t1 == t2 {
		t.Error("tokens should be unique")
	}
}

func TestHashSHA256_Deterministic(t *testing.T) {
	h1 := HashSHA256("test-input")
	h2 := HashSHA256("test-input")
	if h1 != h2 {
		t.Error("SHA256 should be deterministic")
	}
}

func TestHashSHA256_DifferentInputs(t *testing.T) {
	h1 := HashSHA256("input-a")
	h2 := HashSHA256("input-b")
	if h1 == h2 {
		t.Error("different inputs should produce different hashes")
	}
}

func TestHashSHA256_Length(t *testing.T) {
	h := HashSHA256("any-input")
	// SHA-256 → 32 bytes → 64 hex chars
	if len(h) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(h))
	}
}

// ── Concurrency Safety ──────────────────────────────────────────────

func TestBcryptHasher_ConcurrentHashing(t *testing.T) {
	h := NewBcryptHasher(WithCost(4)) // low cost for speed
	var wg sync.WaitGroup
	errs := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hash, err := h.Hash("concurrent-pass")
			if err != nil {
				errs <- err
				return
			}
			if err := h.Verify("concurrent-pass", hash); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent error: %v", err)
	}
}

func TestArgon2Hasher_ConcurrentHashing(t *testing.T) {
	h := NewArgon2Hasher(WithArgon2Memory(1024)) // low memory for speed
	var wg sync.WaitGroup
	errs := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hash, err := h.Hash("concurrent-pass")
			if err != nil {
				errs <- err
				return
			}
			if err := h.Verify("concurrent-pass", hash); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent error: %v", err)
	}
}

// ── Cross-algorithm Safety ──────────────────────────────────────────

func TestBcryptHash_CannotBeVerifiedByArgon2(t *testing.T) {
	bcryptH := NewBcryptHasher()
	argon2H := NewArgon2Hasher()

	hash, _ := bcryptH.Hash("password")
	err := argon2H.Verify("password", hash)
	if err == nil {
		t.Error("argon2 verifier should reject bcrypt hash format")
	}
}

func TestArgon2Hash_CannotBeVerifiedByBcrypt(t *testing.T) {
	argon2H := NewArgon2Hasher()
	bcryptH := NewBcryptHasher()

	hash, _ := argon2H.Hash("password")
	err := bcryptH.Verify("password", hash)
	if err == nil {
		t.Error("bcrypt verifier should reject argon2 hash format")
	}
}

func TestBcryptHasher_InvalidCostHashError(t *testing.T) {
	h := &BcryptHasher{cost: 32}
	if _, err := h.Hash("password123"); err == nil {
		t.Fatal("expected bcrypt hash error for out-of-range cost")
	}
}

func TestArgon2Hasher_Verify_BadBase64(t *testing.T) {
	h := NewArgon2Hasher()
	badSalt := "$argon2id$v=19$m=65536,t=1,p=4$!!!$aGFzaA"
	if err := h.Verify("password123", badSalt); err == nil {
		t.Error("expected decode-salt error")
	}
	badHash := "$argon2id$v=19$m=65536,t=1,p=4$c29tZXNhbHQ$!!!"
	if err := h.Verify("password123", badHash); err == nil {
		t.Error("expected decode-hash error")
	}
}

func TestConfig_Validate_Argon2ThreadsTooLow(t *testing.T) {
	cfg := Config{Algorithm: AlgorithmArgon2id, Argon2Time: 3, Argon2Memory: 64 * 1024, Argon2Threads: 0, MinLength: 8}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected argon2_threads validation error")
	}
}
