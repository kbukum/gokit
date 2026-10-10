package jwt

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"math/big"
	"testing"
)

func TestKeyMaterialRejectsMalformedValues(t *testing.T) {
	t.Parallel()
	var rsaNil *rsa.PrivateKey
	var ecNil *ecdsa.PrivateKey
	for name, tc := range map[string]struct {
		method SigningMethod
		key    Key
	}{
		"nil RSA signer":   {RS256, Key{Signer: rsaNil}},
		"nil EC signer":    {ES256, Key{Signer: ecNil}},
		"nil RSA public":   {RS256, Key{Public: (*rsa.PublicKey)(nil)}},
		"RSA nil N":        {RS256, Key{Public: &rsa.PublicKey{E: 65537}}},
		"RSA invalid E":    {RS256, Key{Public: &rsa.PublicKey{N: new(big.Int).Lsh(big.NewInt(1), 2048), E: 0}}},
		"empty EC":         {ES256, Key{Public: &ecdsa.PublicKey{}}},
		"empty RSA":        {RS256, Key{Signer: &rsa.PrivateKey{}}},
		"EC nil X":         {ES256, Key{Public: &ecdsa.PublicKey{Curve: elliptic.P256(), Y: big.NewInt(1)}}}, //nolint:staticcheck // SA1019: builds the malformed key the guard rejects.
		"EC nil Y":         {ES256, Key{Public: &ecdsa.PublicKey{Curve: elliptic.P256(), X: big.NewInt(1)}}}, //nolint:staticcheck // SA1019: builds the malformed key the guard rejects.
		"EC nil coords":    {ES256, Key{Public: &ecdsa.PublicKey{Curve: elliptic.P256()}}},
		"EC signer nil X":  {ES256, Key{Signer: &ecdsa.PrivateKey{PublicKey: ecdsa.PublicKey{Curve: elliptic.P256()}, D: big.NewInt(1)}}}, //nolint:staticcheck // SA1019: builds the malformed key the guard rejects.
		"EC signer nil D":  {ES256, Key{Signer: validECPublicOnly(t)}},
		"EC signer zero D": {ES256, Key{Signer: withD(validECPublicOnly(t), big.NewInt(0))}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := keyMaterial(tc.method, &tc.key); err == nil {
				t.Fatal("malformed key accepted")
			}
		})
	}
}

func TestKeyMaterialOwnsEd25519Snapshots(t *testing.T) {
	t.Parallel()
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	verify, signing, err := keyMaterial(EdDSA, &Key{Signer: private, Public: pub})
	if err != nil {
		t.Fatal(err)
	}
	pub[0] ^= 1
	private[ed25519.SeedSize] ^= 1
	if verify.(ed25519.PublicKey).Equal(pub) || signing.(crypto.Signer).Public().(ed25519.PublicKey).Equal(private.Public()) {
		t.Fatal("key ring borrows caller-owned key material")
	}
}

func validECPublicOnly(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &ecdsa.PrivateKey{PublicKey: private.PublicKey}
}

func withD(private *ecdsa.PrivateKey, d *big.Int) *ecdsa.PrivateKey {
	private.D = d //nolint:staticcheck // SA1019: builds the malformed key the guard rejects.
	return private
}

func TestKeySnapshotsRejectInconsistentKeys(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherEC, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	nilPrime := *rsaKey
	nilPrime.Primes = []*big.Int{rsaKey.Primes[0], nil}
	wrongExponent := *rsaKey
	wrongExponent.D = new(big.Int).Add(rsaKey.D, big.NewInt(2))
	mismatched := *ecKey
	mismatched.PublicKey = otherEC.PublicKey
	for name, signer := range map[string]struct {
		method SigningMethod
		key    crypto.Signer
	}{
		"unsupported method":   {"HS256", rsaKey},
		"RSA nil prime":        {RS256, &nilPrime},
		"RSA wrong exponent":   {RS256, &wrongExponent},
		"EC mismatched public": {ES256, &mismatched},
	} {
		if _, err := snapshotSigner(signer.method, signer.key); err == nil {
			t.Fatalf("%s: signer accepted", name)
		}
	}
	offCurve := &ecdsa.PublicKey{Curve: elliptic.P256(), X: big.NewInt(1), Y: big.NewInt(1)} //nolint:staticcheck // SA1019: builds the malformed key the guard rejects.
	for name, public := range map[string]struct {
		method SigningMethod
		key    crypto.PublicKey
	}{
		"unsupported method": {"HS256", &rsaKey.PublicKey},
		"EC off curve":       {ES256, offCurve},
	} {
		if _, err := snapshotPublic(public.method, public.key); err == nil {
			t.Fatalf("%s: public key accepted", name)
		}
	}
}
