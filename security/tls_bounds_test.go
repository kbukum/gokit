package security_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kbukum/gokit/fs"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/security/tlstest"
)

func TestTLSConfigBoundsAllExplicitCertificateFiles(t *testing.T) {
	t.Parallel()
	certs := tlstest.GenerateTLSCerts(t)
	path := filepath.Join(t.TempDir(), "oversized.pem")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 1024*1024+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"ca", "certificate", "key"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			cfg := security.TLSConfig{CAFile: certs.CAFile, CertFile: certs.CertFile, KeyFile: certs.KeyFile}
			switch field {
			case "ca":
				cfg.CAFile = path
			case "certificate":
				cfg.CertFile = path
			case "key":
				cfg.KeyFile = path
			}
			if value, err := cfg.Build(); value != nil || !errors.Is(err, fs.ErrFileTooLarge) {
				t.Fatal("oversized certificate material did not retain bounded-reader rejection")
			}
		})
	}
}
