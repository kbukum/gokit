package auth

import "testing"

func TestSigningImplementations(t *testing.T) {
	t.Parallel()

	signers := []Signing{
		GPGSign{KeyID: "ABC123"},
		SSHSign{KeyPath: "id_ed25519.pub"},
	}
	for _, signer := range signers {
		if signer == nil {
			t.Fatal("signer is nil")
		}
		signer.isSigning()
	}
}

func FuzzSigningFields(f *testing.F) {
	f.Add("key")
	f.Fuzz(func(t *testing.T, input string) {
		gpg := GPGSign{KeyID: input}
		ssh := SSHSign{KeyPath: input}
		gpg.isSigning()
		ssh.isSigning()
		if gpg.KeyID != input || ssh.KeyPath != input {
			t.Fatalf("signing config mutated: %#v %#v", gpg, ssh)
		}
	})
}
