package skill

import "errors"

// Sentinel errors for classifiable skill manifest, loading, verification, and
// registry failures. Policy rejections (invalid manifest, parse/size/UTF-8/path
// violations, verification denial, duplicate registration) wrap one of these so
// callers can match with errors.Is; low-level IO failures (missing or unreadable
// files) are returned as-is.
var (
	// ErrManifestInvalid marks a manifest that failed schema validation.
	ErrManifestInvalid = errors.New("skill: invalid manifest")
	// ErrParseManifest marks a manifest that could not be decoded.
	ErrParseManifest = errors.New("skill: manifest parse failed")
	// ErrFileTooLarge marks a pack file that exceeds its size limit.
	ErrFileTooLarge = errors.New("skill: file exceeds size limit")
	// ErrAssetsTooLarge marks a pack whose aggregate assets exceed the limit.
	ErrAssetsTooLarge = errors.New("skill: assets exceed total size limit")
	// ErrInvalidUTF8 marks a text pack file that is not valid UTF-8.
	ErrInvalidUTF8 = errors.New("skill: file is not valid UTF-8")
	// ErrInvalidPackFile marks a pack path that is not an accepted regular
	// file or directory (symlink, escaping path, wrong type).
	ErrInvalidPackFile = errors.New("skill: invalid pack file")
	// ErrVerificationDenied marks a manifest rejected by the verifier.
	ErrVerificationDenied = errors.New("skill: verification denied")
	// ErrAlreadyRegistered marks a duplicate registration.
	ErrAlreadyRegistered = errors.New("skill: already registered")
)
