package testhost

import (
	"crypto/rand"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/security/tlstest"
)

func hostConfig(t *testing.T) Config {
	t.Helper()
	certs := tlstest.GenerateTLSCerts(t)
	return Config{
		Origin: "https://localhost:0", StateFile: filepath.Join(t.TempDir(), "sessions.db"),
		CertFile: certs.CertFile, KeyFile: certs.KeyFile,
		RunID: rand.Text(), BuildID: "test-build",
	}
}

func TestConfigRequiresIsolatedHTTPS(t *testing.T) {
	t.Parallel()
	valid := hostConfig(t)
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"http://localhost:443", "https://example.com:443", "https://localhost", "https://localhost:443/path", "https://localhost:443?query", "https://user@localhost:443", "https://localhost:70000"} {
		t.Run(origin, func(t *testing.T) {
			t.Parallel()
			cfg := valid
			cfg.Origin = origin
			if err := cfg.Validate(); err == nil {
				t.Fatal("invalid host origin accepted")
			}
		})
	}
}

func TestConfigRequiresTrustStateAndRunIdentity(t *testing.T) {
	t.Parallel()
	for _, change := range []func(*Config){
		func(c *Config) { c.CertFile = "" },
		func(c *Config) { c.KeyFile = "" },
		func(c *Config) { c.StateFile = "" },
		func(c *Config) { c.StateFile = ":memory:" },
		func(c *Config) { c.RunID = "" },
		func(c *Config) { c.BuildID = "" },
	} {
		cfg := hostConfig(t)
		change(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Fatal("incomplete fixture config accepted")
		}
	}
}
