package auth

import "testing"

func TestTransportImplementations(t *testing.T) {
	t.Parallel()

	transports := []Transport{
		SSHKey{User: "git", PrivateKeyPath: "id_ed25519"},
		Token{Username: "user", Value: "secret"},
		BasicAuth{Username: "user", Password: "secret"},
		CredentialHelper{Program: "git-credential-store", Args: []string{"get"}},
	}
	for _, transport := range transports {
		if transport == nil {
			t.Fatal("transport is nil")
		}
		transport.isTransport()
	}
}

func FuzzTokenTransport(f *testing.F) {
	f.Add("user", "token")
	f.Fuzz(func(t *testing.T, username, value string) {
		transport := Token{Username: username, Value: value}
		transport.isTransport()
		if transport.Username != username || transport.Value != value {
			t.Fatalf("Token mutated: %#v", transport)
		}
	})
}
