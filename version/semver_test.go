package version

import (
	"errors"
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
)

func TestParseVersion(t *testing.T) {
	t.Parallel()

	v, err := ParseVersion("1.2.3-alpha.1+build.5")
	if err != nil {
		t.Fatalf("ParseVersion: %v", err)
	}
	if v.Major() != 1 || v.Minor() != 2 || v.Patch() != 3 {
		t.Errorf("got %d.%d.%d, want 1.2.3", v.Major(), v.Minor(), v.Patch())
	}

	if _, err := ParseVersion("1.2"); err == nil {
		t.Error("expected partial version to be rejected")
	} else {
		var appErr *apperrors.AppError
		if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeInvalidFormat {
			t.Errorf("expected INVALID_FORMAT, got %v", err)
		}
	}
}

func TestParseRequirement(t *testing.T) {
	t.Parallel()

	c, err := ParseRequirement("^1.2")
	if err != nil {
		t.Fatalf("ParseRequirement: %v", err)
	}
	v, err := ParseVersion("1.3.0")
	if err != nil {
		t.Fatalf("ParseVersion: %v", err)
	}
	if !c.Check(v) {
		t.Error("expected 1.3.0 to satisfy ^1.2")
	}

	if _, err := ParseRequirement("not a requirement"); err == nil {
		t.Error("expected invalid requirement to be rejected")
	}
}

func TestMatchesRequirement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		version     string
		requirement string
		want        bool
		wantErr     bool
	}{
		{"1.3.0", ">=1.2", true, false},
		{"1.1.0", ">=1.2", false, false},
		{"invalid", ">=1.2", false, true},
		{"1.3.0", "invalid", false, true},
	}
	for _, tc := range tests {
		got, err := MatchesRequirement(tc.version, tc.requirement)
		if tc.wantErr {
			if err == nil {
				t.Errorf("MatchesRequirement(%q,%q): expected error", tc.version, tc.requirement)
			}
			continue
		}
		if err != nil {
			t.Errorf("MatchesRequirement(%q,%q): %v", tc.version, tc.requirement, err)
			continue
		}
		if got != tc.want {
			t.Errorf("MatchesRequirement(%q,%q) = %v, want %v", tc.version, tc.requirement, got, tc.want)
		}
	}
}

func TestSupportedSchema(t *testing.T) {
	t.Parallel()

	got, err := SupportedSchema("schema", nil, 1)
	if err != nil || got != 1 {
		t.Errorf("absent = (%v,%v), want (1,nil)", got, err)
	}

	one := 1
	got, err = SupportedSchema("schema", &one, 1)
	if err != nil || got != 1 {
		t.Errorf("matching = (%v,%v), want (1,nil)", got, err)
	}

	two := 2
	_, err = SupportedSchema("schema", &two, 1)
	if err == nil {
		t.Fatal("expected mismatch to be rejected")
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeInvalidInput {
		t.Errorf("expected INVALID_INPUT, got %v", err)
	}
}
