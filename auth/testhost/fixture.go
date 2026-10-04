package testhost

import (
	"crypto/rand"
	"os"

	"github.com/kbukum/gokit/auth/apikey"
	"github.com/kbukum/gokit/codec"
	apperrors "github.com/kbukum/gokit/errors"
	kitfs "github.com/kbukum/gokit/fs"
)

// Fixture holds synthetic secrets for one owned test host. Persist it only in a private, ignored fixture directory; it is not a wire fixture or retained evidence.
type Fixture struct {
	Password     string `json:"password"`
	ControlToken string `json:"controlToken"`
	DigestKey    []byte `json:"digestKey"`
	CSRFKey      []byte `json:"csrfKey"`
	KeyPepper    []byte `json:"keyPepper"`
	APIKey       string `json:"apiKey"`
}

// NewFixture creates independent synthetic credentials and separate digest/CSRF keys.
func NewFixture() (Fixture, error) {
	f := Fixture{
		Password: rand.Text(), ControlToken: rand.Text(),
		DigestKey: make([]byte, 32), CSRFKey: make([]byte, 32),
		KeyPepper: make([]byte, 32),
	}
	for _, key := range [][]byte{f.DigestKey, f.CSRFKey, f.KeyPepper} {
		if _, err := rand.Read(key); err != nil {
			return Fixture{}, apperrors.Internal(err)
		}
		hasher, err := apikey.NewHasher(apikey.HashingConfig{Pepper: string(f.KeyPepper)})
		if err != nil {
			return Fixture{}, err
		}
		key, err := hasher.GenerateKey("fixture")
		if err != nil {
			return Fixture{}, err
		}
		f.APIKey = key.PlainKey
	}
	return f, nil
}

// LoadFixture rejects publicly readable or oversized fixture material. The runner must keep the file private across explicit retained-state restarts.
func LoadFixture(path string) (Fixture, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Fixture{}, apperrors.InvalidInput("fixture", "fixture file is unavailable").WithCause(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return Fixture{}, apperrors.InvalidInput("fixture", "fixture file must be a private regular file")
	}
	data, err := kitfs.ReadFileLimit(path, 4096)
	if err != nil {
		return Fixture{}, err
	}
	f, err := codec.Decode[Fixture](codec.CompactJSON(), string(data))
	if err != nil {
		return Fixture{}, err
	}
	if len(f.Password) < 26 || len(f.Password) > 128 || len(f.ControlToken) < 26 || len(f.ControlToken) > 128 || len(f.DigestKey) != 32 || len(f.CSRFKey) != 32 || len(f.KeyPepper) != 32 || len(f.APIKey) > 512 {
		return Fixture{}, apperrors.InvalidInput("fixture", "fixture requires independent high-entropy credentials and 32-byte keys")
	}
	if _, _, err := apikey.SplitKey(f.APIKey); err != nil {
		return Fixture{}, apperrors.InvalidInput("fixture", "fixture API key is invalid")
	}
	return f, nil
}
