package database

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestConnectionConfigurationRedactsSerializationAndFormatting(t *testing.T) {
	t.Parallel()
	cfg := Config{DSN: "synthetic-private-opaque-input", Params: ConnParams{Host: "db.example", Password: "synthetic-private-password"}}
	for _, value := range []any{cfg, cfg.Params} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		outputs := []string{string(encoded), fmt.Sprintf("%v", value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value)}
		for _, output := range outputs {
			if strings.Contains(output, "synthetic-private") {
				t.Fatal("connection configuration leaked credentials")
			}
		}
	}
	var decoded Config
	if err := json.Unmarshal([]byte(`{"dsn":"opaque","params":{"password":"synthetic"}}`), &decoded); err != nil ||
		decoded.DSN != "opaque" || decoded.Params.Password != "synthetic" {
		t.Fatal("redacted output changed plaintext input decoding")
	}
}
