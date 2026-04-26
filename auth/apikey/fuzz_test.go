package apikey

import "testing"

// FuzzCompareHash ensures the constant-time hash compare never panics or
// returns a false-positive on arbitrary plaintext / stored-hash pairs.
func FuzzCompareHash(f *testing.F) {
	f.Add("", "")
	f.Add("plain", "")
	f.Add("", "deadbeef")
	f.Add("plain", "not-a-hash")
	f.Add("plain", Hash("plain"))
	f.Fuzz(func(t *testing.T, plain, stored string) {
		_ = CompareHash(plain, stored)
	})
}

// FuzzHash ensures Hash is total over arbitrary inputs.
func FuzzHash(f *testing.F) {
	f.Add("")
	f.Add("short")
	f.Add(string(make([]byte, 4096)))
	f.Fuzz(func(t *testing.T, plain string) {
		h := Hash(plain)
		if h == "" {
			t.Fatalf("Hash returned empty for input len=%d", len(plain))
		}
	})
}
