package nats

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	natsgo "github.com/nats-io/nats.go"

	"github.com/kbukum/gokit/security"
)

func TestConfigValidateAndRedactedURL(t *testing.T) {
	t.Parallel()

	cfg := Config{URL: "tls://localhost:4222", SubjectPrefix: "svc.events", QueueGroup: "workers", Token: "secret"}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate nats config: %v", err)
	}
	redacted := cfg.RedactedURL()
	if strings.Contains(redacted, "secret") || strings.Contains(redacted, "user:") {
		t.Fatalf("redacted URL leaked credentials: %q", redacted)
	}
}

func TestConfigValidateRejectsURLCredentialsAndPlaintext(t *testing.T) {
	t.Parallel()

	cfg := Config{URL: "nats://user:secret@localhost:4222", AllowInsecureDev: true}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected URL credential validation error")
	}

	cfg = Config{URL: "nats://localhost:4222"}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected plaintext validation error")
	}
	cfg.AllowInsecureDev = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("allow_insecure_dev should permit plaintext: %v", err)
	}
}

func TestConfigValidateRejectsConflictingAuth(t *testing.T) {
	t.Parallel()

	cfg := Config{URL: "tls://localhost:4222", Token: "token", Username: "user"}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected conflicting auth validation error")
	}
}

func TestConfigDoesNotExposeCoreNameEnabled(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(Config{})
	for _, name := range []string{"Name", "Enabled"} {
		if _, ok := typ.FieldByName(name); ok {
			t.Fatalf("nats.Config exposes core field %s", name)
		}
	}
}

func TestConfigPasswordFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(path, []byte("s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{URL: "tls://localhost:4222", Username: "runs", PasswordFile: path}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	opts, err := cfg.connectOptions()
	if err != nil {
		t.Fatalf("connect options: %v", err)
	}
	var applied natsgo.Options
	for _, opt := range opts {
		if err := opt(&applied); err != nil {
			t.Fatal(err)
		}
	}
	if user, password := applied.UserInfo(); user != "runs" || password != "s3cret" {
		t.Fatalf("user info = %q", user)
	}
	// A rotated password is read at the next connect.
	if err := os.WriteFile(path, []byte("rotated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, password := applied.UserInfo(); password != "rotated" {
		t.Fatal("rotated password not picked up")
	}

	for name, bad := range map[string]Config{
		"both passwords": {URL: cfg.URL, Username: "runs", Password: "x", PasswordFile: path},
		"no username":    {URL: cfg.URL, PasswordFile: path},
		"with token":     {URL: cfg.URL, Token: "t", PasswordFile: path},
	} {
		bad.ApplyDefaults()
		if err := bad.Validate(); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}

	missing := cfg
	missing.PasswordFile = filepath.Join(t.TempDir(), "missing")
	_, err = missing.connectOptions()
	if err == nil {
		t.Fatal("expected missing password file error")
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("error leaked secret: %v", err)
	}
}

// A service with its own inbox prefix receives replies only on subjects its broker user may subscribe to.
func TestConfigInboxPrefix(t *testing.T) {
	t.Parallel()

	cfg := Config{URL: "tls://localhost:4222", InboxPrefix: "_INBOX.runs"}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	opts, err := cfg.connectOptions()
	if err != nil {
		t.Fatal(err)
	}
	var applied natsgo.Options
	for _, opt := range opts {
		if err := opt(&applied); err != nil {
			t.Fatal(err)
		}
	}
	if applied.InboxPrefix != "_INBOX.runs" {
		t.Fatalf("inbox prefix = %q", applied.InboxPrefix)
	}

	for _, bad := range []string{"_INBOX.>", "_INBOX.*", "_INBOX.runs.", ".runs", "_INBOX..runs", "_INBOX runs"} {
		cfg := Config{URL: "tls://localhost:4222", InboxPrefix: bad}
		cfg.ApplyDefaults()
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "inbox prefix") {
			t.Fatalf("%q: validate = %v", bad, err)
		}
	}
}

// A URL names only a server: no path, query or fragment, and a CA applies only to a TLS URL.
func TestConfigValidateRejectsURLExtras(t *testing.T) {
	t.Parallel()

	for _, ok := range []string{"tls://broker:4222", "tls://broker:4222/", "tls://a:4222, tls://b:4222"} {
		cfg := Config{URL: ok}
		cfg.ApplyDefaults()
		if err := cfg.Validate(); err != nil {
			t.Fatalf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"tls://broker:4222/path", "tls://broker:4222?x=1", "tls://broker:4222#frag"} {
		cfg := Config{URL: bad}
		cfg.ApplyDefaults()
		if err := cfg.Validate(); err == nil {
			t.Fatalf("%q: want error", bad)
		}
	}
	cfg := Config{URL: "nats://broker:4222", AllowInsecureDev: true, TLS: &security.TLSConfig{CAFile: "ca.pem"}}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "ca_file") {
		t.Fatalf("CA with a cleartext url: %v", err)
	}
}

// UserInbox gives each broker user the reply inbox "_INBOX.<username>".
func TestConfigUserInbox(t *testing.T) {
	t.Parallel()

	cfg := Config{URL: "tls://localhost:4222", Username: "runs", Password: "p", UserInbox: true}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	opts, err := cfg.connectOptions()
	if err != nil {
		t.Fatal(err)
	}
	var applied natsgo.Options
	for _, opt := range opts {
		if err := opt(&applied); err != nil {
			t.Fatal(err)
		}
	}
	if applied.InboxPrefix != "_INBOX.runs" {
		t.Fatalf("inbox prefix = %q", applied.InboxPrefix)
	}
	for name, bad := range map[string]Config{
		"no username":  {URL: "tls://localhost:4222", Token: "t", UserInbox: true},
		"both inboxes": {URL: "tls://localhost:4222", Username: "runs", Password: "p", UserInbox: true, InboxPrefix: "_INBOX.x"},
		"bad username": {URL: "tls://localhost:4222", Username: "a.>", Password: "p", UserInbox: true},
	} {
		bad.ApplyDefaults()
		if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "inbox") {
			t.Fatalf("%s: validate = %v", name, err)
		}
	}
}
