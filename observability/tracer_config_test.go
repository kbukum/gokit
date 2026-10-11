package observability

import (
	"net/url"
	"strings"
	"testing"
)

func FuzzTracerConfigEndpoint(f *testing.F) {
	for _, endpoint := range []string{
		"localhost:4318", "[::1]:4318", "collector:+4318", "collector%zz:4318",
		"private-user@collector:4318", "collector:4318/path?private-token=value", "",
	} {
		f.Add(endpoint)
	}
	f.Fuzz(func(t *testing.T, endpoint string) {
		cfg := DefaultTracerConfig("test")
		cfg.Endpoint = endpoint
		_, err := normalizedTracerConfig(&cfg)
		if err != nil {
			if strings.Contains(err.Error(), "private-") {
				t.Fatal("endpoint input leaked into configuration error")
			}
			return
		}
		u, err := url.Parse("https://" + endpoint)
		if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			t.Fatal("accepted endpoint cannot form a safe collector URL")
		}
	})
}
