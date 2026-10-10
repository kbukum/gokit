package jwt

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"fmt"
	"math/big"
)

func snapshotPublic(method SigningMethod, key crypto.PublicKey) (crypto.PublicKey, error) {
	switch method {
	case RS256:
		pub, ok := key.(*rsa.PublicKey)
		if !ok || pub == nil || pub.N == nil || pub.N.Sign() <= 0 || pub.N.BitLen() < minRSABits || pub.N.Bit(0) != 1 || pub.E < 3 || pub.E%2 == 0 || pub.E >= 1<<31 {
			return nil, fmt.Errorf("RS256 requires a valid RSA key of at least %d bits", minRSABits)
		}
		return &rsa.PublicKey{N: new(big.Int).Set(pub.N), E: pub.E}, nil
	case ES256:
		pub, ok := key.(*ecdsa.PublicKey)
		if !ok || pub == nil || pub.Curve != elliptic.P256() || !ecdsaCoordinatesSet(pub) {
			return nil, fmt.Errorf("ES256 requires a valid P-256 key")
		}
		encoded, err := pub.Bytes()
		if err != nil {
			return nil, fmt.Errorf("ECDSA public key: %w", err)
		}
		return ecdsa.ParseUncompressedPublicKey(elliptic.P256(), encoded)
	case EdDSA:
		pub, ok := key.(ed25519.PublicKey)
		if !ok || len(pub) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("EdDSA requires an Ed25519 key")
		}
		return ed25519.PublicKey(bytes.Clone(pub)), nil
	default:
		return nil, fmt.Errorf("unsupported signing method")
	}
}

func snapshotSigner(method SigningMethod, key crypto.Signer) (crypto.Signer, error) {
	switch method {
	case RS256:
		private, ok := key.(*rsa.PrivateKey)
		if !ok || private == nil || private.D == nil || private.D.Sign() <= 0 || len(private.Primes) < 2 {
			return nil, fmt.Errorf("RS256 signer must be a valid *rsa.PrivateKey")
		}
		pub, err := snapshotPublic(method, &private.PublicKey)
		if err != nil {
			return nil, err
		}
		ownedPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("RSA snapshot has an invalid key type")
		}
		owned := &rsa.PrivateKey{PublicKey: *ownedPub, D: new(big.Int).Set(private.D), Primes: make([]*big.Int, len(private.Primes))}
		for i, prime := range private.Primes {
			if prime == nil || prime.Sign() <= 0 {
				return nil, fmt.Errorf("RSA private key has an invalid prime")
			}
			owned.Primes[i] = new(big.Int).Set(prime)
		}
		if err := owned.Validate(); err != nil {
			return nil, fmt.Errorf("RSA private key: %w", err)
		}
		owned.Precompute()
		return owned, nil
	case ES256:
		private, ok := key.(*ecdsa.PrivateKey)
		if !ok || private == nil || private.Curve != elliptic.P256() || !ecdsaScalarSet(private) {
			return nil, fmt.Errorf("ES256 signer must be a valid *ecdsa.PrivateKey")
		}
		pub, err := snapshotPublic(method, &private.PublicKey)
		if err != nil {
			return nil, err
		}
		encoded, err := private.Bytes()
		if err != nil {
			return nil, fmt.Errorf("ECDSA private key: %w", err)
		}
		owned, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), encoded)
		if err != nil {
			return nil, fmt.Errorf("ECDSA private key: %w", err)
		}
		if !owned.PublicKey.Equal(pub) {
			return nil, fmt.Errorf("ECDSA public key does not match private key")
		}
		return owned, nil
	case EdDSA:
		private, ok := key.(ed25519.PrivateKey)
		if !ok || len(private) != ed25519.PrivateKeySize || !bytes.Equal(private, ed25519.NewKeyFromSeed(private[:ed25519.SeedSize])) {
			return nil, fmt.Errorf("EdDSA signer must be a valid ed25519.PrivateKey")
		}
		return ed25519.PrivateKey(bytes.Clone(private)), nil
	default:
		return nil, fmt.Errorf("unsupported signing method")
	}
}

// ecdsaCoordinatesSet reports whether a hand-built ECDSA public key carries both coordinates. Every crypto/ecdsa
// encoding method still dereferences them without a nil check, so reading the deprecated fields is the only way to
// reject such a key with an error instead of a panic.
func ecdsaCoordinatesSet(pub *ecdsa.PublicKey) bool {
	return pub.X != nil && pub.Y != nil //nolint:staticcheck // SA1019: nil guard before PublicKey.Bytes, which panics on nil coordinates.
}

// ecdsaScalarSet reports whether a hand-built ECDSA private key carries its scalar. PrivateKey.Bytes rejects a zero
// scalar but panics on a nil one.
func ecdsaScalarSet(private *ecdsa.PrivateKey) bool {
	return private.D != nil //nolint:staticcheck // SA1019: nil guard before PrivateKey.Bytes, which panics on a nil scalar.
}
