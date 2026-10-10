package password

import "testing"

func BenchmarkVerificationEnvelope(b *testing.B) {
	for name, cfg := range map[string]Config{
		"argon2id-issued":  {},
		"argon2id-maximum": {Argon2Memory: maxArgon2Memory, Argon2Time: maxArgon2Time, Argon2Threads: 8},
		"bcrypt-maximum":   {Algorithm: AlgorithmBcrypt, BcryptCost: 14},
	} {
		b.Run(name, func(b *testing.B) {
			hasher, err := NewHasher(cfg)
			if err != nil {
				b.Fatal(err)
			}
			hash, err := hasher.Hash("benchmark-password")
			if err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			b.ReportAllocs()
			for range b.N {
				if err := hasher.Verify("benchmark-password", hash); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
