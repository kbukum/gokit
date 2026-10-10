package password

import (
	"encoding/base64"
	"fmt"
	"strings"
)

func validateBcryptEncoding(hash string, limit int) error {
	if len(hash) > limit || len(hash) != 60 || hash[0] != '$' || hash[1] != '2' || !strings.ContainsRune("aby", rune(hash[2])) ||
		hash[3] != '$' || hash[6] != '$' || hash[4] < '0' || hash[4] > '9' || hash[5] < '0' || hash[5] > '9' {
		return fmt.Errorf("password: malformed bcrypt encoding")
	}
	encoding := base64.NewEncoding("./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789").WithPadding(base64.NoPadding).Strict()
	if _, err := encoding.DecodeString(hash[7:29]); err != nil {
		return fmt.Errorf("password: malformed bcrypt salt: %w", err)
	}
	if _, err := encoding.DecodeString(hash[29:]); err != nil {
		return fmt.Errorf("password: malformed bcrypt checksum: %w", err)
	}
	return nil
}
