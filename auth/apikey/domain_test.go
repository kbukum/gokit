package apikey

import (
	"strings"
	"testing"
)

func TestCredentialDigestDomainSeparation(t *testing.T) {
	api, err := NewHasher(HashingConfig{Pepper: strings.Repeat("p", 32)})
	if err != nil {
		t.Fatal(err)
	}
	session, err := NewHasher(HashingConfig{Pepper: strings.Repeat("p", 32), Domain: "session"})
	if err != nil {
		t.Fatal(err)
	}
	value := "same-credential-material"
	if api.Digest(value) == session.Digest(value) {
		t.Fatal("credential domains collided")
	}
	if api.Compare(value, session.Digest(value)) || session.Compare(value, api.Digest(value)) {
		t.Fatal("cross-domain digest accepted")
	}
	if !api.Compare(value, api.Digest(value)) || !session.Compare(value, session.Digest(value)) {
		t.Fatal("matching domain failed")
	}
	for _, domain := range []string{"contains space", "nul\x00domain", strings.Repeat("a", 65)} {
		if _, err := NewHasher(HashingConfig{Pepper: strings.Repeat("p", 32), Domain: domain}); err == nil {
			t.Fatal("invalid domain accepted")
		}
	}
}
