package testutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/database"
)

func TestOpenWithSharedServerNeverFallsBackToAContainer(t *testing.T) {
	t.Setenv(EnvAddr, "127.0.0.1:1")
	t.Setenv(EnvPasswordFile, "")
	t.Setenv("DOCKER_HOST", "://invalid")
	fixture, err := Open(t.Context())
	if err == nil || fixture != nil {
		t.Fatalf("unreachable shared server certified a fixture: %v", err)
	}
}

func TestOpenRejectsInvalidSharedSettings(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	for name, env := range map[string]map[string]string{
		"no port":          {EnvAddr: "localhost"},
		"bad port":         {EnvAddr: "localhost:99999"},
		"no host":          {EnvAddr: ":5432"},
		"missing password": {EnvAddr: "localhost:5432", EnvPasswordFile: missing},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(EnvPasswordFile, "")
			for k, v := range env {
				t.Setenv(k, v)
			}
			if _, err := Open(t.Context()); err == nil {
				t.Fatal("invalid shared settings accepted")
			}
		})
	}
}

func TestEnvParamsReadsUserAndPasswordFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(file, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvUser, "admin")
	t.Setenv(EnvPasswordFile, file)
	params, err := envParams("db.local:6543")
	if err != nil {
		t.Fatal(err)
	}
	if params.Host != "db.local" || params.Port != 6543 || params.User != "admin" || params.Password != "secret" ||
		params.Database != "postgres" || params.Options["sslmode"] != "disable" || !params.AllowInsecureDev {
		t.Fatalf("unexpected params: host=%s port=%d user=%s db=%s", params.Host, params.Port, params.User, params.Database)
	}
	t.Setenv(EnvUser, "")
	if params, err = envParams("db.local:6543"); err != nil || params.User != "postgres" {
		t.Fatalf("default user: %s %v", params.User, err)
	}
}

func TestConnectRejectsInvalidInput(t *testing.T) {
	valid := database.ConnParams{Host: "localhost", Port: 5432, User: "postgres", Database: "postgres"}
	var nilCtx context.Context
	if _, err := Connect(nilCtx, valid); err == nil {
		t.Fatal("nil context accepted")
	}
	for name, params := range map[string]database.ConnParams{
		"host": {Port: 5432, User: "postgres", Database: "postgres"},
		"port": {Host: "localhost", User: "postgres", Database: "postgres"},
		"user": {Host: "localhost", Port: 5432, Database: "postgres"},
		"db":   {Host: "localhost", Port: 5432, User: "postgres"},
	} {
		if _, err := Connect(t.Context(), params); err == nil {
			t.Fatalf("missing %s accepted", name)
		}
	}
}
