package di

import (
	"fmt"
	"net/http"

	apperr "github.com/kbukum/gokit/errors"
)

// errNilContainer is the typed error returned when a container operation is
// called on a nil *Container.
func errNilContainer() error {
	return apperr.New(apperr.ErrCodeInvalidInput, "di: container is nil", http.StatusUnprocessableEntity)
}

// errNilConstructor is the typed error returned when a registration is given a
// nil constructor for key k.
func errNilConstructor(k typeKey) error {
	return apperr.New(apperr.ErrCodeInvalidInput,
		fmt.Sprintf("di: constructor for %s must not be nil", k), http.StatusUnprocessableEntity)
}

// errNilDisposer is the typed error returned when a closeable registration is
// given a nil disposer for key k.
func errNilDisposer(k typeKey) error {
	return apperr.New(apperr.ErrCodeInvalidInput,
		fmt.Sprintf("di: disposer for %s must not be nil", k), http.StatusUnprocessableEntity)
}
