package session

import (
	"context"
	"strings"
	"testing"

	"github.com/kbukum/gokit/auth/apikey"
)

func TestSessionManagerUsesSeparateCredentialDomain(t *testing.T) {
	manager, _ := fixture(t, &memoryStore{rows: make(map[string]Record)})
	issued, err := manager.Create(context.Background(), caller())
	if err != nil {
		t.Fatal(err)
	}
	api, err := apikey.NewHasher(apikey.HashingConfig{Pepper: strings.Repeat("p", 32)})
	if err != nil {
		t.Fatal(err)
	}
	if api.Digest(issued.Token) == issued.Principal.Reference {
		t.Fatal("session reused API-key credential domain")
	}
	protected, err := apikey.NewHasher(apikey.HashingConfig{Pepper: strings.Repeat("p", 32), Domain: "session"})
	if err != nil {
		t.Fatal(err)
	}
	if protected.Digest(issued.Token) != issued.Principal.Reference {
		t.Fatal("unexpected session credential protection")
	}
}
