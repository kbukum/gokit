package version

import (
	"github.com/Masterminds/semver/v3"

	apperrors "github.com/kbukum/gokit/errors"
)

// ParseVersion parses a strict semantic version string (MAJOR.MINOR.PATCH with
// optional pre-release and build metadata). Partial versions such as "1.2" are
// rejected. The returned error preserves the underlying parse cause.
func ParseVersion(value string) (*semver.Version, error) {
	v, err := semver.StrictNewVersion(value)
	if err != nil {
		return nil, apperrors.InvalidFormat("version", "semantic version").WithCause(err)
	}
	return v, nil
}

// ParseRequirement parses a semantic version requirement such as ">=1.2",
// "^1.2", or "1.2.x". The returned error preserves the underlying parse cause.
func ParseRequirement(requirement string) (*semver.Constraints, error) {
	c, err := semver.NewConstraint(requirement)
	if err != nil {
		return nil, apperrors.InvalidFormat("requirement", "semantic version requirement").WithCause(err)
	}
	return c, nil
}

// MatchesRequirement reports whether version satisfies requirement. It returns
// an error when either input cannot be parsed.
func MatchesRequirement(version, requirement string) (bool, error) {
	v, err := ParseVersion(version)
	if err != nil {
		return false, err
	}
	c, err := ParseRequirement(requirement)
	if err != nil {
		return false, err
	}
	return c.Check(v), nil
}
