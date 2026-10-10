package password

import "testing"

func TestGenerateTokenLengthAndUniqueness(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool, 100)
	for range 100 {
		token, err := GenerateToken(32)
		if err != nil {
			t.Fatalf("GenerateToken: %v", err)
		}
		if len(token) != 64 || seen[token] {
			t.Fatalf("token %q: wrong length or duplicate", token)
		}
		seen[token] = true
	}
}

func TestHashSHA256(t *testing.T) {
	t.Parallel()
	a, b := HashSHA256("input-a"), HashSHA256("input-b")
	if a != HashSHA256("input-a") || a == b || len(a) != 64 {
		t.Fatalf("HashSHA256: %q %q", a, b)
	}
}
