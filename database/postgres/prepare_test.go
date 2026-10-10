package postgres

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"
	"testing"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/util"
)

func paramsForPreparation() database.ConnParams {
	return database.ConnParams{Host: "db.example", User: "application", Password: "synthetic", Database: "application", Options: map[string]string{"search_path": "application"}}
}

func TestPrepareOwnsVerifiedConfigurationAndFreshPools(t *testing.T) {
	t.Parallel()
	params := paramsForPreparation()
	prepared, err := Prepare(t.Context(), database.ConnectionInput{Params: params})
	if err != nil {
		t.Fatal(err)
	}
	o, ok := prepared.(*opener)
	if !ok {
		t.Fatal("unexpected prepared factory")
	}
	params.Options["search_path"] = "changed"
	cfg := o.config
	if cfg.Host != "db.example" || cfg.Port != 5432 || cfg.Password != "synthetic" || strings.Contains(cfg.ConnString(), "synthetic") {
		t.Fatal("configuration lost explicit endpoint or retained a secret connection string")
	}
	if cfg.TLSConfig == nil || cfg.TLSConfig.InsecureSkipVerify || cfg.TLSConfig.MinVersion != tls.VersionTLS13 ||
		cfg.TLSConfig.ServerName != "db.example" || len(cfg.Fallbacks) != 0 {
		t.Fatal("verified TLS/host/fallback configuration is inconsistent")
	}
	if cfg.RuntimeParams["search_path"] != `"application", pg_temp` {
		t.Fatal("application schema and explicit last pg_temp were not retained")
	}
	first, err := o.Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	second, err := o.Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	a, aOK := first.(*ownedDialector)
	b, bOK := second.(*ownedDialector)
	if !aOK || !bOK {
		t.Fatal("opener returned an unowned pool")
	}
	t.Cleanup(func() {
		if err := errors.Join(a.Close(), b.Close()); err != nil {
			t.Error(err)
		}
	})
	if a.pool == b.pool {
		t.Fatal("retry factory reused a pool")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if value, err := o.Open(ctx); value != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("canceled factory opened a pool")
	}
}

func TestPrepareRejectsUnsupportedConnectionPolicy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ key, value string }{
		{"search_path", "public"},
		{"search_path", "pg_catalog"},
		{"search_path", "app, attacker"},
		{"search_path", ""},
		{"search_path", "_app"},
		{"search_path", "App"},
		{"sslmode", "prefer"},
		{"sslmode", "allow"},
		{"sslmode", "require"},
		{"passfile", "untrusted"},
		{"service", "untrusted"},
		{"options", "untrusted"},
		{"password", "untrusted"},
		{"statement_timeout", "0"},
		{"statement_timeout", "300001"},
		{"idle_in_transaction_session_timeout", "-1"},
		{"application_name", strings.Repeat("x", 64)},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			t.Parallel()
			params := paramsForPreparation()
			params.Options[tc.key] = tc.value
			if value, err := Prepare(t.Context(), database.ConnectionInput{Params: params}); err == nil || value != nil {
				t.Fatal("unsafe policy prepared a connection")
			}
		})
	}
	if _, err := Prepare(t.Context(), database.ConnectionInput{DSN: "postgres://user:synthetic@localhost/app?search_path=app"}); err == nil || strings.Contains(err.Error(), "synthetic") {
		t.Fatal("credential URL accepted or leaked")
	}
	if _, err := Prepare(t.Context(), database.ConnectionInput{DSN: "postgres://user@localhost/app?search_path=app"}); err != nil {
		t.Fatal(err)
	}
}

type countingSecrets struct{ calls int }

func (s *countingSecrets) ReadSecret(context.Context, string) (util.SecretString, error) {
	s.calls++
	return util.NewSecretString("synthetic"), nil
}

func TestPrepareRejectsAmbientBeforeSecretOrDriverAccess(t *testing.T) {
	for _, key := range []string{"PGPASSWORD", "PGPASSFILE", "PGSERVICE", "PGSERVICEFILE", "PGSSLROOTCERT", "PGSSLCERT", "PGSSLKEY", "PGSSLMODE", "PGOPTIONS"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "untrusted")
			source := &countingSecrets{}
			params := paramsForPreparation()
			params.Password, params.PasswordFile = "", "injected"
			if value, err := Prepare(t.Context(), database.ConnectionInput{Params: params, Secrets: source}); value != nil || err == nil || source.calls != 0 {
				t.Fatal("ambient input reached credential/driver access")
			}
		})
	}
}

func TestPrepareRejectsInvalidInputAtEachBoundary(t *testing.T) {
	t.Parallel()
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	var nilCtx context.Context
	if _, err := Prepare(nilCtx, database.ConnectionInput{Params: paramsForPreparation()}); err == nil {
		t.Fatal("nil context accepted")
	}
	if _, err := Prepare(canceled, database.ConnectionInput{Params: paramsForPreparation()}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context = %v", err)
	}
	for name, dsn := range map[string]string{
		"unparsable":        "postgres://user@local host/app",
		"wrong scheme":      "mysql://user@localhost/app?search_path=app",
		"no user":           "postgres://localhost/app?search_path=app",
		"fragment":          "postgres://user@localhost/app?search_path=app#x",
		"overflowing port":  "postgres://user@localhost:99999999999999999999/app?search_path=app",
		"malformed options": "postgres://user@localhost/app?search_path=%zz",
		"repeated option":   "postgres://user@localhost/app?search_path=a&search_path=b",
	} {
		if value, err := Prepare(t.Context(), database.ConnectionInput{DSN: dsn}); value != nil || err == nil {
			t.Errorf("%s DSN prepared a connection", name)
		}
	}
	for name, mutate := range map[string]func(*database.ConnParams){
		"no structured input": func(p *database.ConnParams) { *p = database.ConnParams{} },
		"missing host":        func(p *database.ConnParams) { p.Host = "" },
		"multiple hosts":      func(p *database.ConnParams) { p.Host = "a,b" },
		"negative port":       func(p *database.ConnParams) { p.Port = -1 },
		"port above range":    func(p *database.ConnParams) { p.Port = 65536 },
		"missing user":        func(p *database.ConnParams) { p.User = "" },
		"oversized database":  func(p *database.ConnParams) { p.Database = strings.Repeat("d", 64) },
		"oversized option":    func(p *database.ConnParams) { p.Options["application_name"] = strings.Repeat("x", 4097) },
		"NUL option":          func(p *database.ConnParams) { p.Options["application_name"] = "a\x00b" },
		"unknown timezone":    func(p *database.ConnParams) { p.Options["timezone"] = "Mars/Phobos" },
		"root CA without TLS": func(p *database.ConnParams) { p.Options["sslmode"], p.Options["sslrootcert"] = "disable", "ca.pem" },
		"unreadable root CA": func(p *database.ConnParams) {
			p.Options["sslrootcert"] = t.TempDir() + "/missing.pem"
		},
		"unreadable password file": func(p *database.ConnParams) {
			p.Password, p.PasswordFile = "", t.TempDir()+"/missing"
		},
	} {
		params := paramsForPreparation()
		mutate(&params)
		if value, err := Prepare(t.Context(), database.ConnectionInput{Params: params}); value != nil || err == nil {
			t.Errorf("%s prepared a connection", name)
		}
	}
}

func TestPrepareAppliesBoundedSessionOptions(t *testing.T) {
	t.Parallel()
	params := paramsForPreparation()
	params.Port = 6543
	for key, value := range map[string]string{
		"sslmode": "disable", "statement_timeout": "1500", "idle_in_transaction_session_timeout": "2500",
		"application_name": "worker", "timezone": "Europe/Istanbul",
	} {
		params.Options[key] = value
	}
	prepared, err := Prepare(t.Context(), database.ConnectionInput{Params: params})
	if err != nil {
		t.Fatal(err)
	}
	cfg := prepared.(*opener).config
	runtime := cfg.RuntimeParams
	if cfg.Port != 6543 || cfg.TLSConfig != nil || runtime["statement_timeout"] != "1500" ||
		runtime["idle_in_transaction_session_timeout"] != "2500" || runtime["application_name"] != "worker" ||
		runtime["timezone"] != "Europe/Istanbul" || prepared.(*opener).location.String() != "Europe/Istanbul" {
		t.Fatalf("session options were not applied: port=%d tls=%v runtime=%v", cfg.Port, cfg.TLSConfig != nil, runtime)
	}
}
