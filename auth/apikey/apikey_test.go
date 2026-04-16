package apikey

import "testing"

func TestCompareHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		plainKey string
		hash     string
		want     bool
	}{
		{
			name:     "matching key and hash",
			plainKey: "sk_live_abc123",
			hash:     Hash("sk_live_abc123"),
			want:     true,
		},
		{
			name:     "non-matching key",
			plainKey: "sk_live_wrong",
			hash:     Hash("sk_live_abc123"),
			want:     false,
		},
		{
			name:     "empty plaintext and empty hash",
			plainKey: "",
			hash:     Hash(""),
			want:     true,
		},
		{
			name:     "empty plaintext vs non-empty hash",
			plainKey: "",
			hash:     Hash("sk_live_abc123"),
			want:     false,
		},
		{
			name:     "non-empty plaintext vs empty hash string",
			plainKey: "sk_live_abc123",
			hash:     "",
			want:     false,
		},
		{
			name:     "hash of key always matches",
			plainKey: "sk_test_0123456789abcdef",
			hash:     Hash("sk_test_0123456789abcdef"),
			want:     true,
		},
		{
			name:     "case sensitive",
			plainKey: "SK_LIVE_ABC123",
			hash:     Hash("sk_live_abc123"),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CompareHash(tt.plainKey, tt.hash)
			if got != tt.want {
				t.Errorf("CompareHash(%q, hash) = %v, want %v", tt.plainKey, got, tt.want)
			}
		})
	}
}

func TestCompareHash_GeneratedKey(t *testing.T) {
	t.Parallel()

	// CompareHash(key, Hash(key)) must always be true for generated keys.
	result, err := Generate("sk_live_")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if !CompareHash(result.PlainKey, result.KeyHash) {
		t.Error("CompareHash should return true for a generated key and its hash")
	}
}

func TestCompareHash_RejectsModifiedKey(t *testing.T) {
	t.Parallel()

	result, err := Generate("sk_live_")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	modified := result.PlainKey + "x"
	if CompareHash(modified, result.KeyHash) {
		t.Error("CompareHash should return false for a modified key")
	}
}
