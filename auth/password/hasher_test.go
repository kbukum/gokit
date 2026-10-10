package password

import (
	"strings"
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
)

func algorithms(t *testing.T) map[Algorithm]Hasher {
	t.Helper()
	return map[Algorithm]Hasher{
		AlgorithmArgon2id: newHasher(t, Config{MinLength: 4, MaxLength: 20}),
		AlgorithmBcrypt:   newHasher(t, Config{Algorithm: AlgorithmBcrypt, MinLength: 4, MaxLength: 20}),
	}
}

func TestHasherRoundTripAndTypedFailures(t *testing.T) {
	t.Parallel()
	for algorithm, h := range algorithms(t) {
		t.Run(string(algorithm), func(t *testing.T) {
			t.Parallel()
			hash, err := h.Hash("pässwörd")
			if err != nil {
				t.Fatalf("Hash: %v", err)
			}
			again, err := h.Hash("pässwörd")
			if err != nil || again == hash {
				t.Fatalf("salt reuse or error: %v", err)
			}
			if err := h.Verify("pässwörd", hash); err != nil {
				t.Fatalf("Verify: %v", err)
			}
			mismatch := h.Verify("pässwörD", hash)
			requireError(t, mismatch, apperrors.ErrCodeUnauthorized, ReasonMismatch)
			if strings.Contains(mismatch.Error(), "pässw") {
				t.Fatal("error leaks the password")
			}
			requireError(t, h.Verify(strings.Repeat("x", 21), hash), apperrors.ErrCodeUnauthorized, ReasonMismatch)
			requireError(t, h.Verify("pässwörd", "not-a-hash"), apperrors.ErrCodeInternal, ReasonCorruptHash)
		})
	}
}

func TestHasherEnforcesConfiguredLengthInCharacters(t *testing.T) {
	t.Parallel()
	for algorithm, h := range algorithms(t) {
		t.Run(string(algorithm), func(t *testing.T) {
			t.Parallel()
			requireError(t, hashErr(h, "äöü"), apperrors.ErrCodeInvalidInput, ReasonTooShort)
			if err := hashErr(h, "äöüß"); err != nil {
				t.Fatalf("4 characters / 8 bytes rejected: %v", err)
			}
			if err := hashErr(h, strings.Repeat("a", 20)); err != nil {
				t.Fatalf("maximum rejected: %v", err)
			}
			requireError(t, hashErr(h, strings.Repeat("a", 21)), apperrors.ErrCodeInvalidInput, ReasonTooLong)
		})
	}
	bcrypt := newHasher(t, Config{Algorithm: AlgorithmBcrypt, MinLength: 4})
	requireError(t, hashErr(bcrypt, strings.Repeat("ä", 37)), apperrors.ErrCodeInvalidInput, ReasonTooLong)
}

func hashErr(h Hasher, password string) error {
	_, err := h.Hash(password)
	return err
}

func TestHashesAreNotInterchangeable(t *testing.T) {
	t.Parallel()
	hashers := algorithms(t)
	argon, bcrypt := hashers[AlgorithmArgon2id], hashers[AlgorithmBcrypt]
	argonHash, err := argon.Hash("password")
	if err != nil {
		t.Fatal(err)
	}
	bcryptHash, err := bcrypt.Hash("password")
	if err != nil {
		t.Fatal(err)
	}
	requireError(t, argon.Verify("password", bcryptHash), apperrors.ErrCodeInternal, ReasonCorruptHash)
	requireError(t, bcrypt.Verify("password", argonHash), apperrors.ErrCodeInternal, ReasonCorruptHash)
}

func TestArgon2VerifyRejectsHostileParameters(t *testing.T) {
	t.Parallel()
	h := newHasher(t, Config{MinLength: 4})
	hash, err := h.Hash("password")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(hash, "$")
	if parts[3] != "m=65536,t=3,p=4" {
		t.Fatalf("encoded parameters %q", parts[3])
	}
	replace := func(index int, value string) string {
		p := append([]string(nil), parts...)
		p[index] = value
		return strings.Join(p, "$")
	}
	for name, corrupt := range map[string]string{
		"version":        replace(2, "v=18"),
		"threads":        replace(3, "m=65536,t=3,p=0"),
		"trailing":       replace(3, "m=65536,t=3,p=4,x"),
		"salt":           replace(4, "!!"),
		"short salt":     replace(4, "AAAA"),
		"key":            replace(5, "!!"),
		"short key":      replace(5, "AAAA"),
		"long salt":      replace(4, strings.Repeat("A", 90)),
		"oversized hash": replace(5, strings.Repeat("A", 600)),
		"missing fields": "$argon2id$v=19$m=65536,t=3,p=4$salt",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			requireError(t, h.Verify("password", corrupt), apperrors.ErrCodeInternal, ReasonCorruptHash)
		})
	}
}

func TestVerifyRejectsUnsupportedCostBeforeKDF(t *testing.T) {
	t.Parallel()
	h := newHasher(t, Config{MinLength: 4})
	const salt = "c2FsdHNhbHRzYWx0c2FsdA"
	const key = "aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGFzaGhhc2g"
	for _, params := range []string{"m=4194304,t=3,p=4", "m=65536,t=1000,p=4", "m=65536,t=3,p=9", "m=65537,t=3,p=4"} {
		requireError(t, h.Verify("password", "$argon2id$v=19$"+params+"$"+salt+"$"+key), apperrors.ErrCodeInternal, ReasonUnsupportedCost)
	}
	b := newHasher(t, Config{Algorithm: AlgorithmBcrypt})
	hash := "$2a$31$" + strings.Repeat(".", 53)
	requireError(t, b.Verify("password", hash), apperrors.ErrCodeInternal, ReasonUnsupportedCost)
}

func FuzzArgon2Verify(f *testing.F) {
	h, err := NewHasher(Config{MinLength: 1})
	if err != nil {
		f.Fatal(err)
	}
	f.Add("password", "$argon2id$v=19$m=65536,t=3,p=4$c2FsdHNhbHRzYWx0c2FsdA$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGFzaGhhc2g")
	f.Fuzz(func(t *testing.T, password, hash string) {
		if err := h.Verify(password, hash); err == nil {
			t.Fatal("fuzzed hash verified")
		}
	})
}
