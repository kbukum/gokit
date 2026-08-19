package encryption

import (
	"encoding/base64"
	"fmt"
	"net/http"

	apperrors "github.com/kbukum/gokit/errors"
)

// Ciphertext envelope layout (before base64):
//
//	version[1] || algorithm[1] || salt[saltSize] || nonce[nonceSize] || ciphertext
//
// The header (version, algorithm id, salt, nonce) is authenticated as AEAD
// associated data, binding the algorithm and key-derivation inputs to the
// sealed body. This format is wire-compatible with rskit's encryption envelope.
const (
	envelopeVersion    byte = 1
	nonceSize               = 12
	aeadTagSize             = 16
	envelopeHeaderSize      = 1 + 1 + saltSize + nonceSize
)

// id returns the stable on-wire algorithm identifier.
func (a Algorithm) id() (byte, bool) {
	switch a {
	case AlgorithmAESGCM:
		return 1, true
	case AlgorithmChaCha20:
		return 2, true
	default:
		return 0, false
	}
}

// algorithmFromID maps an on-wire identifier back to its Algorithm.
func algorithmFromID(id byte) (Algorithm, bool) {
	switch id {
	case 1:
		return AlgorithmAESGCM, true
	case 2:
		return AlgorithmChaCha20, true
	default:
		return "", false
	}
}

func invalidEnvelope(message string) *apperrors.AppError {
	return apperrors.New(apperrors.ErrCodeInvalidFormat, message, http.StatusUnprocessableEntity)
}

// envelope is a decoded, minimally validated ciphertext envelope.
type envelope struct {
	data []byte
}

// header returns the authenticated header bytes for the given inputs.
func envelopeHeader(alg Algorithm, salt, nonce []byte) ([]byte, error) {
	id, ok := alg.id()
	if !ok {
		return nil, apperrors.InvalidInput("algorithm", fmt.Sprintf("unsupported algorithm %q", alg))
	}
	header := make([]byte, 0, envelopeHeaderSize)
	header = append(header, envelopeVersion, id)
	header = append(header, salt...)
	header = append(header, nonce...)
	return header, nil
}

// encodeEnvelope assembles and base64-encodes header || ciphertext.
func encodeEnvelope(header, ciphertext []byte) string {
	data := make([]byte, 0, len(header)+len(ciphertext))
	data = append(data, header...)
	data = append(data, ciphertext...)
	return base64.StdEncoding.EncodeToString(data)
}

// decodeEnvelope decodes and minimally validates a ciphertext envelope.
func decodeEnvelope(ciphertext string) (*envelope, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, invalidEnvelope("ciphertext is not valid base64").WithCause(err)
	}
	if len(data) < envelopeHeaderSize+aeadTagSize {
		return nil, invalidEnvelope("ciphertext envelope is too short")
	}
	if data[0] != envelopeVersion {
		return nil, invalidEnvelope("unsupported ciphertext envelope version")
	}
	if _, ok := algorithmFromID(data[1]); !ok {
		return nil, invalidEnvelope("unsupported ciphertext algorithm")
	}
	return &envelope{data: data}, nil
}

func (e *envelope) algorithm() Algorithm {
	alg, _ := algorithmFromID(e.data[1])
	return alg
}

func (e *envelope) associatedData() []byte { return e.data[:envelopeHeaderSize] }

func (e *envelope) salt() []byte { return e.data[2 : 2+saltSize] }

func (e *envelope) nonce() []byte { return e.data[2+saltSize : envelopeHeaderSize] }

func (e *envelope) ciphertext() []byte { return e.data[envelopeHeaderSize:] }
