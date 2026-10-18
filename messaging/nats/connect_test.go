package nats

import (
	"testing"

	natsgo "github.com/nats-io/nats.go"
)

func TestConnectValidatesAndAppliesExtraOptions(t *testing.T) {
	t.Parallel()
	if _, err := Connect(Config{URL: "nats://localhost:4222"}); err == nil {
		t.Fatal("cleartext URL accepted without allow_insecure_dev")
	}
	if _, err := Connect(Config{URL: "nats://127.0.0.1:1", AllowInsecureDev: true}); err == nil {
		t.Fatal("connect to a closed port succeeded")
	}
	conn, err := Connect(Config{URL: "nats://127.0.0.1:1", AllowInsecureDev: true}, natsgo.RetryOnFailedConnect(true))
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
}
