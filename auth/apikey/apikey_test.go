package apikey

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/kbukum/gokit/auth"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

func testHasher(t *testing.T) *Hasher {
	t.Helper()
	hasher, err := NewHasher(HashingConfig{Pepper: strings.Repeat("p", 32)})
	if err != nil {
		t.Fatalf("NewHasher: %v", err)
	}
	return hasher
}

func TestHashingConfigValidate(t *testing.T) {
	t.Parallel()

	if _, err := NewHasher(HashingConfig{Pepper: "short"}); err == nil {
		t.Fatal("expected short pepper to fail")
	}
	if _, err := NewHasher(HashingConfig{Pepper: strings.Repeat("p", 32), EntropyBytes: 8}); err == nil {
		t.Fatal("expected short entropy to fail")
	}
}

func TestHasherGenerateAndCompare(t *testing.T) {
	t.Parallel()

	hasher := testHasher(t)
	issued, err := hasher.GenerateKey("sk_live")
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if issued.KeyPrefix != "sk_live" {
		t.Fatalf("KeyPrefix = %q, want sk_live", issued.KeyPrefix)
	}
	if !strings.HasPrefix(issued.PlainKey, "sk_live.") {
		t.Fatalf("PlainKey = %q, want sk_live prefix", issued.PlainKey)
	}
	if !hasher.Compare(issued.PlainKey, issued.KeyDigest) {
		t.Fatal("expected digest comparison to succeed")
	}
	if hasher.Compare(issued.PlainKey+"x", issued.KeyDigest) {
		t.Fatal("expected modified key to fail comparison")
	}
}

func TestHasherConfigRedactsPepper(t *testing.T) {
	t.Parallel()

	hasher := testHasher(t)
	cfg := hasher.Config()
	if cfg.Pepper != "" {
		t.Fatalf("Config() leaked pepper: %q", cfg.Pepper)
	}
	if cfg.EntropyBytes != defaultEntropyBytes {
		t.Fatalf("EntropyBytes = %d, want %d", cfg.EntropyBytes, defaultEntropyBytes)
	}
}

func TestSplitKey(t *testing.T) {
	t.Parallel()

	prefix, secret, err := SplitKey("sk.secret")
	if err != nil {
		t.Fatalf("SplitKey: %v", err)
	}
	if prefix != "sk" || secret != "secret" {
		t.Fatalf("SplitKey returned %q %q", prefix, secret)
	}
	if _, _, err := SplitKey("malformed"); err == nil {
		t.Fatal("expected malformed key to fail")
	}
}

func TestRequestAuthentication(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, nil, nil)
	issued, _, err := manager.IssueKey(context.Background(), IssueRequest{KeyID: "key-1", OwnerID: "user-1", Name: "primary", Prefix: "pkg", Scopes: nil, ExpiresAt: nil})
	if err != nil {
		t.Fatalf("IssueKey: %v", err)
	}
	chain := auth.NewChain(nil, manager)

	t.Run("reports genuinely absent credentials without an identity", func(t *testing.T) {
		t.Parallel()
		p, present, err := chain.Authenticate(httptest.NewRequest(http.MethodGet, "/", http.NoBody))
		if err != nil || present || p.Subject != "" {
			t.Fatalf("absent credentials: principal=%+v present=%v err=%v", p, present, err)
		}
	})

	t.Run("authenticates a valid key as a typed principal", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.Header.Set("X-API-Key", issued.PlainKey)
		p, present, err := chain.Authenticate(req)
		if err != nil || !present || p.Subject != "user-1" || p.Credential != auth.APIKey || p.Reference != "key-1" {
			t.Fatalf("valid key: principal=%+v err=%v", p, err)
		}
	})

	rejected := map[string]func(*http.Request){
		"invalid key": func(r *http.Request) { r.Header.Set("X-API-Key", "pk.invalid") },
		"empty key":   func(r *http.Request) { r.Header["X-Api-Key"] = []string{""} },
		"duplicate key fields": func(r *http.Request) {
			r.Header["X-Api-Key"] = []string{issued.PlainKey, issued.PlainKey}
		},
		"case-aliased key fields": func(r *http.Request) {
			r.Header["X-Api-Key"] = []string{issued.PlainKey}
			r.Header["x-api-key"] = []string{issued.PlainKey}
		},
		"mixed key and session cookie": func(r *http.Request) {
			r.Header.Set("X-API-Key", issued.PlainKey)
			r.Header.Set("Cookie", auth.SessionCookie+"=opaque")
		},
		"key in authorization header": func(r *http.Request) { r.Header.Set("Authorization", issued.PlainKey) },
	}
	for name, mutate := range rejected {
		t.Run("rejects "+name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			mutate(req)
			_, present, err := chain.Authenticate(req)
			if err == nil || !present || apperrors.Normalize(err).Code != apperrors.ErrCodeUnauthorized {
				t.Fatalf("expected unauthorized, got %v", err)
			}
		})
	}
}

func TestValidatePrefixRejectsInvalid(t *testing.T) {
	t.Parallel()
	hasher := testHasher(t)
	cases := []string{"", "ab", "bad/prefix", "bad prefix"}
	for _, prefix := range cases {
		if _, err := hasher.GenerateKey(prefix); apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
			t.Fatalf("GenerateKey(%q) expected prefix rejection, got %v", prefix, err)
		}
	}
}

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

func TestEntropyFailureIsInternalNotInvalidPrefix(t *testing.T) {
	t.Parallel()
	hasher, err := NewHasher(HashingConfig{Pepper: strings.Repeat("p", 32), Random: iotest.ErrReader(errors.New("entropy exhausted"))})
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(Config{Store: newMemory(t), Hasher: hasher, Clock: util.NewFakeClock(epoch)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = hasher.GenerateKey("sk_live")
	if got := apperrors.Normalize(err); got.Code != apperrors.ErrCodeInternal {
		t.Fatalf("hasher entropy failure classified as %v", got.Code)
	}
	if _, _, err := m.IssueKey(context.Background(), IssueRequest{KeyID: "k1", OwnerID: "o", Prefix: "sk_live", Kind: auth.User, Scopes: []string{"read"}}); apperrors.Normalize(err).Code != apperrors.ErrCodeInternal {
		t.Fatalf("manager entropy failure classified as %v", apperrors.Normalize(err).Code)
	}
}

func TestKeyValidAt(t *testing.T) {
	t.Parallel()
	expires, grace := epoch.Add(time.Hour), epoch.Add(time.Minute)
	cases := map[string]struct {
		key  Key
		now  time.Time
		want bool
	}{
		"no expiry":            {Key{}, epoch, true},
		"before expiry":        {Key{ExpiresAt: &expires}, expires.Add(-time.Nanosecond), true},
		"at expiry":            {Key{ExpiresAt: &expires}, expires, false},
		"within grace":         {Key{ExpiresAt: &expires, GraceEndsAt: &grace}, grace.Add(-time.Nanosecond), true},
		"at grace end":         {Key{ExpiresAt: &expires, GraceEndsAt: &grace}, grace, false},
		"revoked":              {Key{RevokedAt: &epoch}, epoch.Add(-time.Hour), false},
		"revoked within grace": {Key{GraceEndsAt: &grace, RevokedAt: &epoch}, epoch, false},
	}
	for name, tc := range cases {
		if got := tc.key.ValidAt(tc.now); got != tc.want {
			t.Fatalf("%s: ValidAt = %v, want %v", name, got, tc.want)
		}
	}
	if (*Key)(nil).ValidAt(epoch) {
		t.Fatal("nil key valid")
	}
}
