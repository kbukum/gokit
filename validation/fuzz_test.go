package validation

import "testing"

func FuzzValidateUUID(f *testing.F) {
	f.Add("550e8400-e29b-41d4-a716-446655440000")
	f.Add("not-a-uuid")
	f.Fuzz(func(t *testing.T, value string) {
		_, _ = ValidateUUID(value, "id")
	})
}
