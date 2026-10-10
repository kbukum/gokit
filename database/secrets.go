package database

import (
	"context"
	"strings"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/fs"
	"github.com/kbukum/gokit/util"
)

const maxSecretBytes = 4096

// SecretSource resolves an explicitly configured secret during connection preparation.
type SecretSource interface {
	ReadSecret(context.Context, string) (util.SecretString, error)
}

// FileSecrets follows mounted-secret symlinks and reads bounded regular files. Filesystem operations are synchronous, not universally cancelable.
type FileSecrets struct{}

func (FileSecrets) ReadSecret(ctx context.Context, path string) (util.SecretString, error) {
	if util.IsNil(ctx) {
		return util.SecretString{}, apperrors.InvalidInput("context", "Context is required")
	}
	if err := ctx.Err(); err != nil {
		return util.SecretString{}, err
	}
	data, err := fs.ReadFileLimit(path, maxSecretBytes)
	if err != nil {
		return util.SecretString{}, Failure(err)
	}
	if err := ctx.Err(); err != nil {
		return util.SecretString{}, err
	}
	value := strings.TrimSuffix(string(data), "\n")
	if len(data) != len(value) {
		value = strings.TrimSuffix(value, "\r")
	}
	secret := util.NewSecretString(value)
	if err := validateSecret(secret); err != nil {
		return util.SecretString{}, err
	}
	return secret, nil
}

// ResolvePassword validates exclusivity and resolves only the selected explicit credential source.
func ResolvePassword(ctx context.Context, params ConnParams, source SecretSource) (util.SecretString, error) {
	if util.IsNil(ctx) {
		return util.SecretString{}, apperrors.InvalidInput("context", "Context is required")
	}
	if err := ctx.Err(); err != nil {
		return util.SecretString{}, err
	}
	if params.Password != "" && params.PasswordFile != "" {
		return util.SecretString{}, apperrors.InvalidInput("password", "Password and password_file are mutually exclusive")
	}
	if params.PasswordFile == "" {
		secret := util.NewSecretString(params.Password)
		if !secret.IsEmpty() {
			if err := validateSecret(secret); err != nil {
				return util.SecretString{}, err
			}
		}
		return secret, nil
	}
	if source == nil {
		source = FileSecrets{}
	} else if util.IsNil(source) {
		return util.SecretString{}, apperrors.InvalidInput("secret_source", "Secret source must not be typed nil")
	}
	secret, err := source.ReadSecret(ctx, params.PasswordFile)
	if err != nil {
		return util.SecretString{}, Failure(err)
	}
	if err := ctx.Err(); err != nil {
		return util.SecretString{}, err
	}
	if err := validateSecret(secret); err != nil {
		return util.SecretString{}, err
	}
	return secret, nil
}

func validateSecret(secret util.SecretString) error {
	if secret.IsEmpty() || secret.Len() > maxSecretBytes || strings.ContainsRune(secret.Expose(), '\x00') {
		return apperrors.InvalidInput("password", "Secret must contain 1 to 4096 bytes without NUL")
	}
	return nil
}
