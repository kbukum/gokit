package wire

import (
	"math"
	"slices"
	"strings"
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
)

func TestFixtureNamesMatchPublishedCases(t *testing.T) {
	t.Parallel()
	names := FixtureNames()
	published, err := LoadFixtures()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.IsSorted(names) || len(names) != len(published) || len(names) != len(WireCases()) {
		t.Fatal("fixture inventory drift")
	}
	for i, name := range names {
		if _, ok := published[name]; !ok {
			t.Fatalf("missing fixture %q", name)
		}
		if i > 0 && name == names[i-1] {
			t.Fatalf("duplicate fixture %q", name)
		}
	}
}

func TestFixtureRenderingRejectsInvalidWireData(t *testing.T) {
	t.Parallel()
	for _, failure := range []*apperrors.AppError{
		apperrors.Internal(nil).WithDetail("badNumber", math.NaN()),
		apperrors.Internal(nil).WithReason("\xff"),
		apperrors.Validation("bad field").WithViolations(apperrors.Violation{Field: "\xff", Reason: apperrors.ViolationReason("INVALID_FORMAT"), Message: "invalid"}),
		apperrors.InvalidInput("frame", strings.Repeat("x", 128<<10)),
	} {
		if _, err := RenderFixture(WireCase{Name: "invalid", Err: failure}); err == nil {
			t.Fatal("invalid fixture rendered as success")
		}
	}
}
