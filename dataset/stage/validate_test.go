package stage

import (
	"errors"
	"testing"
)

func TestValidatorFunc(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("rejected")
	var v Validator[int] = ValidatorFunc[int](func(n int) error {
		if n < 0 {
			return sentinel
		}
		return nil
	})
	if err := v.Validate(1); err != nil {
		t.Errorf("Validate(1) = %v; want nil", err)
	}
	if err := v.Validate(-1); !errors.Is(err, sentinel) {
		t.Errorf("Validate(-1) = %v; want sentinel", err)
	}
}
