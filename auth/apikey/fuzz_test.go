package apikey

import "testing"

func FuzzSplitKey(f *testing.F) {
	f.Add("pk.secret")
	f.Add("malformed")
	f.Fuzz(func(t *testing.T, plain string) {
		_, _, _ = SplitKey(plain)
	})
}

func FuzzDigestCompare(f *testing.F) {
	hasher, err := NewHasher(HashingConfig{Pepper: "pppppppppppppppppppppppppppppppp"})
	if err != nil {
		f.Fatalf("NewHasher: %v", err)
	}
	f.Add("pk.secret")
	f.Fuzz(func(t *testing.T, plain string) {
		digest := hasher.Digest(plain)
		_ = hasher.Compare(plain, digest)
	})
}
