package oci

import (
	"regexp"
	"strings"

	apperr "github.com/kbukum/gokit/errors"
)

// Patterns from the OCI distribution specification; digests are limited to sha256, which the client verifies.
var (
	repositoryPattern = regexp.MustCompile(`^[a-z0-9]+(?:(?:\.|_|__|-+)[a-z0-9]+)*(?:/[a-z0-9]+(?:(?:\.|_|__|-+)[a-z0-9]+)*)*$`)
	tagPattern        = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$`)
	digestPattern     = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

const maxRepositoryBytes = 255

// Reference names a model artifact in a registry: a repository and either a tag or a digest.
type Reference struct {
	// Repository is the repository path, such as "ai/smollm2".
	Repository string
	// Tag is the mutable tag, such as "latest". Empty when Digest pins the reference.
	Tag string
	// Digest pins the manifest, such as "sha256:…".
	Digest string
}

// ParseReference parses "repository[:tag]" or "repository@sha256:…" relative to the client's registry. A missing tag
// means "latest".
func ParseReference(s string) (Reference, error) {
	var ref Reference
	if repo, digest, ok := strings.Cut(s, "@"); ok {
		ref = Reference{Repository: repo, Digest: digest}
	} else if i := strings.LastIndex(s, ":"); i >= 0 && !strings.Contains(s[i:], "/") {
		ref = Reference{Repository: s[:i], Tag: s[i+1:]}
	} else {
		ref = Reference{Repository: s, Tag: "latest"}
	}
	if err := ref.Validate(); err != nil {
		return Reference{}, err
	}
	return ref, nil
}

// Validate checks the repository and exactly one of tag or digest.
func (r Reference) Validate() error {
	if err := validRepository(r.Repository); err != nil {
		return err
	}
	switch {
	case r.Digest != "" && r.Tag != "":
		return apperr.InvalidInput("reference", "Use a tag or a digest, not both")
	case r.Digest != "":
		return validDigest(r.Digest)
	case !tagPattern.MatchString(r.Tag):
		return apperr.InvalidInput("tag", "must be a valid OCI tag")
	}
	return nil
}

// String formats the reference as "repository:tag" or "repository@digest".
func (r Reference) String() string {
	if r.Digest != "" {
		return r.Repository + "@" + r.Digest
	}
	return r.Repository + ":" + r.Tag
}

func (r Reference) ref() string {
	if r.Digest != "" {
		return r.Digest
	}
	return r.Tag
}

func validRepository(repo string) error {
	if repo == "" || len(repo) > maxRepositoryBytes || !repositoryPattern.MatchString(repo) {
		return apperr.InvalidInput("repository", "must be a valid OCI repository path")
	}
	return nil
}

func validDigest(d string) error {
	if !digestPattern.MatchString(d) {
		return apperr.InvalidInput("digest", "must be a sha256 digest")
	}
	return nil
}
