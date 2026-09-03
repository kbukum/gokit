package postgres_test

import (
	"testing"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/postgres"
)

func TestDialect_DSN(t *testing.T) {
	t.Parallel()

	sd, ok := postgres.Dialect().(database.StructuredDialect)
	if !ok {
		t.Fatal("postgres.Dialect() must implement database.StructuredDialect")
	}

	tests := []struct {
		name   string
		params database.ConnParams
		want   string
	}{
		{
			name:   "defaults for port and sslmode",
			params: database.ConnParams{Host: "db.example", User: "u", Password: "p", Database: "app"},
			want:   "postgres://u:p@db.example:5432/app?sslmode=verify-full",
		},
		{
			name: "explicit port and sslmode preserved",
			params: database.ConnParams{
				Host:     "db.example",
				Port:     6543,
				User:     "u",
				Password: "p",
				Database: "app",
				Options:  map[string]string{"sslmode": "require"},
			},
			want: "postgres://u:p@db.example:6543/app?sslmode=require",
		},
		{
			name:   "password slash is url-escaped",
			params: database.ConnParams{Host: "db.example", User: "user", Password: "p/w", Database: "app"},
			want:   "postgres://user:p%2Fw@db.example:5432/app?sslmode=verify-full",
		},
		{
			name:   "ipv6 host is bracketed",
			params: database.ConnParams{Host: "::1", User: "u", Password: "p", Database: "app"},
			want:   "postgres://u:p@[::1]:5432/app?sslmode=verify-full",
		},
		{
			name: "reserved characters in database and options are encoded",
			params: database.ConnParams{
				Host:     "db.example",
				User:     "u",
				Password: "s3cr3t",
				Database: "my db",
				Options:  map[string]string{"application_name": "svc x"},
			},
			want: "postgres://u:s3cr3t@db.example:5432/my%20db?application_name=svc+x&sslmode=verify-full",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := sd.DSN(tc.params)
			if err != nil {
				t.Fatalf("DSN() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Errorf("DSN() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDialect_DSN_RejectsInvalidParams(t *testing.T) {
	t.Parallel()

	sd := postgres.Dialect().(database.StructuredDialect)

	tests := []struct {
		name   string
		params database.ConnParams
	}{
		{name: "empty host", params: database.ConnParams{User: "u", Password: "p", Database: "app"}},
		{name: "negative port", params: database.ConnParams{Host: "db.example", Port: -1, Database: "app"}},
		{name: "port above range", params: database.ConnParams{Host: "db.example", Port: 70000, Database: "app"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got, err := sd.DSN(tc.params); err == nil {
				t.Fatalf("DSN() = %q, want error for %s", got, tc.name)
			}
		})
	}
}
