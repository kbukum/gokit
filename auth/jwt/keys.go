package jwt

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
)

const (
	maxKeys           = 8
	maxKeyIDBytes     = 128
	minHMACSecretSize = 32
	minRSABits        = 2048
)

// Key is one verification key and, optionally, its signing half. Exactly one kind of material is set: Secret for HS256,
// or Signer and/or Public for asymmetric methods. Signer must be the standard library private key type for the method
// (*rsa.PrivateKey, *ecdsa.PrivateKey or ed25519.PrivateKey). Public is derived from Signer when omitted.
type Key struct {
	// ID is the "kid" header value: 1..128 bytes of [A-Za-z0-9._:-].
	ID string

	// Signer is the private key used for issuance.
	Signer crypto.Signer

	// Public is the verification key for asymmetric methods.
	Public crypto.PublicKey

	// Secret is the HS256 key, at least 32 bytes.
	Secret []byte

	// RetireAt stops verification with this key at the given instant. Zero means the key does not retire.
	RetireAt time.Time
}

// KeySet holds the keys a Service verifies with and selects the key it signs with. Rotation publishes the new key, moves
// signing to it, and keeps the old key with a RetireAt no earlier than the last token it signed expires. An empty
// SigningKeyID makes the service verify-only.
type KeySet struct {
	SigningKeyID string
	Keys         []Key
}

type verificationKey struct {
	key      any
	retireAt time.Time
}

type keyRing struct {
	signingID string
	signing   any
	verify    map[string]verificationKey
}

func newKeyRing(method SigningMethod, set KeySet) (keyRing, error) {
	if len(set.Keys) == 0 || len(set.Keys) > maxKeys {
		return keyRing{}, fmt.Errorf("key set must hold 1..%d keys", maxKeys)
	}
	ring := keyRing{signingID: set.SigningKeyID, verify: make(map[string]verificationKey, len(set.Keys))}
	unbounded := 0
	for i := range set.Keys {
		key := &set.Keys[i]
		if !validKeyID(key.ID) {
			return keyRing{}, fmt.Errorf("key %d: id must be 1..%d bytes of [A-Za-z0-9._:-]", i, maxKeyIDBytes)
		}
		if _, dup := ring.verify[key.ID]; dup {
			return keyRing{}, fmt.Errorf("key %q is listed twice", key.ID)
		}
		verify, signing, err := keyMaterial(method, key)
		if err != nil {
			return keyRing{}, fmt.Errorf("key %q: %w", key.ID, err)
		}
		if key.RetireAt.IsZero() {
			unbounded++
		}
		ring.verify[key.ID] = verificationKey{key: verify, retireAt: key.RetireAt}
		if key.ID == set.SigningKeyID {
			if signing == nil {
				return keyRing{}, fmt.Errorf("signing key %q has no private material", key.ID)
			}
			if !key.RetireAt.IsZero() {
				return keyRing{}, fmt.Errorf("signing key %q must not retire", key.ID)
			}
			ring.signing = signing
		}
	}
	if unbounded > 1 {
		return keyRing{}, fmt.Errorf("at most one key may omit retire_at")
	}
	if set.SigningKeyID != "" && ring.signing == nil {
		return keyRing{}, fmt.Errorf("signing key %q is not in the key set", set.SigningKeyID)
	}
	return ring, nil
}

func validKeyID(id string) bool {
	if id == "" || len(id) > maxKeyIDBytes {
		return false
	}
	return !strings.ContainsFunc(id, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && !strings.ContainsRune("._:-", r)
	})
}

// keyMaterial returns the verification key and the signing key (nil when verify-only) in the types golang-jwt expects.
func keyMaterial(method SigningMethod, key *Key) (verify, signing any, err error) {
	if method == HS256 {
		if key.Signer != nil || key.Public != nil {
			return nil, nil, fmt.Errorf("HS256 keys hold only a secret")
		}
		if len(key.Secret) < minHMACSecretSize {
			return nil, nil, fmt.Errorf("HS256 secret must be at least %d bytes", minHMACSecretSize)
		}
		secret := bytes.Clone(key.Secret)
		return secret, secret, nil
	}
	if key.Secret != nil {
		return nil, nil, fmt.Errorf("%s keys do not take a secret", method)
	}
	public := key.Public
	if key.Signer != nil {
		private, copyErr := snapshotSigner(method, key.Signer)
		if copyErr != nil {
			return nil, nil, copyErr
		}
		signing = private
		derived := private.Public()
		if public == nil {
			public = derived
		} else {
			validated, copyErr := snapshotPublic(method, public)
			if copyErr != nil {
				return nil, nil, copyErr
			}
			eq, ok := derived.(interface{ Equal(crypto.PublicKey) bool })
			if !ok || !eq.Equal(validated) {
				return nil, nil, fmt.Errorf("public key does not match signer")
			}
			public = validated
		}
	}
	if public == nil {
		return nil, nil, fmt.Errorf("key material is required")
	}
	verify, err = snapshotPublic(method, public)
	return verify, signing, err
}

// ParseKey reads one PEM block: a PKCS #8 "PRIVATE KEY" yields a signing key, a PKIX "PUBLIC KEY" a verify-only key.
// Anything after the block other than whitespace is rejected. Method-specific checks run when the key set is built.
func ParseKey(id string, data []byte) (Key, error) {
	invalid := func(cause error) (Key, error) {
		return Key{}, apperrors.InvalidInput("jwt.key", "Invalid key material").WithCause(cause)
	}
	block, rest := pem.Decode(data)
	if block == nil {
		return invalid(fmt.Errorf("key %q: no PEM block found", id))
	}
	if len(bytes.TrimSpace(rest)) != 0 {
		return invalid(fmt.Errorf("key %q: expected exactly one PEM block", id))
	}
	switch block.Type {
	case "PRIVATE KEY":
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return invalid(fmt.Errorf("key %q: parse PKCS #8 private key: %w", id, err))
		}
		signer, ok := parsed.(crypto.Signer)
		if !ok {
			return invalid(fmt.Errorf("key %q: private key cannot sign", id))
		}
		return Key{ID: id, Signer: signer}, nil
	case "PUBLIC KEY":
		parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return invalid(fmt.Errorf("key %q: parse PKIX public key: %w", id, err))
		}
		return Key{ID: id, Public: parsed}, nil
	default:
		return invalid(fmt.Errorf("key %q: unsupported PEM block %q", id, block.Type))
	}
}
