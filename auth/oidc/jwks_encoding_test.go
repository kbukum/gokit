package oidc

import (
	"bytes"
	"crypto/elliptic"
	"encoding/base64"
	"testing"
)

func TestECJWKFixedWidthCoordinates(t *testing.T) {
	t.Parallel()
	for name, curve := range map[string]elliptic.Curve{
		"P-256": elliptic.P256(), "P-384": elliptic.P384(), "P-521": elliptic.P521(),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			size := (curve.Params().BitSize + 7) / 8
			x := curve.Params().Gx.FillBytes(make([]byte, size))
			y := curve.Params().Gy.FillBytes(make([]byte, size))
			key := &jwk{
				Kty: "EC", Crv: name,
				X: base64.RawURLEncoding.EncodeToString(x),
				Y: base64.RawURLEncoding.EncodeToString(y),
			}
			pub, err := key.ecPublicKey()
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := pub.Bytes()
			if err != nil || !bytes.Equal(encoded[1:1+size], x) || !bytes.Equal(encoded[1+size:], y) {
				t.Fatalf("key did not preserve fixed-width coordinates: %v", err)
			}
			key.X = base64.RawURLEncoding.EncodeToString(append([]byte{0}, x...))
			if _, err := key.ecPublicKey(); err == nil {
				t.Fatal("accepted oversized coordinate")
			}
		})
	}
}
